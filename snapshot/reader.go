// Package snapshot validates and streams the GOPHGRPH binary format.
package snapshot

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"gophergraph/snapshot/ports"
	"math"
	"runtime"
)

var ErrCorruptSnapshot = errors.New("corrupt snapshot")
var ErrUnsupportedFormat = errors.New("unsupported snapshot format")
var ErrUnsupportedPlatform = errors.New("unsupported snapshot platform")
var strides = [24]uint64{8, 1, 4, 8, 4, 8, 16, 4, 4, 4, 4, 8, 16, 8, 4, 4, 8, 4, 4, 24, 4, 4, 32, 4}
var le = binary.LittleEndian

func corrupt(s string) error { return fmt.Errorf("%w: %s", ErrCorruptSnapshot, s) }
func Open(ctx context.Context, source ports.Source) (g *graph.Graph, err error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if source == nil {
		return nil, errors.New("nil snapshot source")
	}
	back, e := source.Acquire(ctx)
	if e != nil {
		return nil, e
	}
	if back == nil {
		return nil, errors.New("source returned nil backing")
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, back.Close())
		}
	}()
	return openBacking(ctx, back)
}
func openBacking(ctx context.Context, back ports.Backing) (*graph.Graph, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return nil, ErrUnsupportedPlatform
	}
	b := back.Bytes()
	if len(b) < 832 {
		return nil, corrupt("short header/directory")
	}
	if string(b[:8]) != "GOPHGRPH" {
		return nil, corrupt("magic")
	}
	if le.Uint32(b[8:]) != 1 {
		return nil, ErrUnsupportedFormat
	}
	size := uint64(len(b))
	if le.Uint32(b[12:]) != 64 || le.Uint64(b[16:]) > 1 || le.Uint64(b[24:]) != size || le.Uint32(b[52:]) != 24 || le.Uint64(b[56:]) != 64 {
		return nil, corrupt("header")
	}
	n, e, s := le.Uint64(b[32:]), le.Uint64(b[40:]), uint64(le.Uint32(b[48:]))
	if n > math.MaxUint32 || e > math.MaxUint32 || s == 0 {
		return nil, corrupt("counts")
	}
	var sections [24][]byte
	var counts [24]uint64
	next := uint64(832)
	for i, stride := range strides {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := b[64+i*32:]
		offset, length, count := le.Uint64(entry[8:]), le.Uint64(entry[16:]), le.Uint64(entry[24:])
		if le.Uint32(entry) != uint32(i+1) || uint64(le.Uint32(entry[4:])) != stride || offset != next || offset > size || count > (size-offset)/stride || count*stride != length {
			return nil, corrupt("section interval")
		}
		end := offset + length
		if end > math.MaxInt64-63 {
			return nil, corrupt("alignment overflow")
		}
		next = (end + 63) &^ 63
		if next > size {
			return nil, corrupt("padding bounds")
		}
		if !bytes.Equal(b[end:next], make([]byte, next-end)) {
			return nil, corrupt("nonzero padding")
		}
		sections[i] = b[offset:end]
		counts[i] = count
	}
	if next != size {
		return nil, corrupt("trailing bytes")
	}
	for i, want := range map[int]uint64{0: s + 1, 2: n, 3: n + 1, 5: n + 1, 7: e, 8: e, 9: e, 10: e, 11: e + 1, 13: n + 1, 14: e, 15: e, 16: n + 1, 17: e, 18: e} {
		if counts[i] != want {
			return nil, corrupt("section count")
		}
	}
	for _, index := range []int{6, 12, 22} {
		raw, stride := sections[index], int(strides[index])
		for at := 0; at < len(raw); at += stride {
			if at%4096 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			if index == 22 {
				if raw[at+2] != 0 || raw[at+3] != 0 {
					return nil, corrupt("property index reserved")
				}
			} else if raw[at+5] != 0 || raw[at+6] != 0 || raw[at+7] != 0 {
				return nil, corrupt("property reserved")
			}
		}
	}
	u32 := func(i int) graphdata.U32 { return graphdata.U32{Bytes: sections[i-1]} }
	u64 := func(i int) graphdata.U64 { return graphdata.U64{Bytes: sections[i-1]} }
	d := &graphdata.Data{StringOffsets: u64(1), StringBytes: sections[1], NodeIDs: u32(3), NodeLabelOffsets: u64(4), NodeLabels: u32(5), NodePropOffsets: u64(6), NodeProps: graphdata.Properties{Bytes: sections[6]}, EdgeIDs: u32(8), Sources: u32(9), Targets: u32(10), EdgeLabels: u32(11), EdgePropOffsets: u64(12), EdgeProps: graphdata.Properties{Bytes: sections[12]}, ForwardOffsets: u64(14), ForwardNeighbors: u32(15), ForwardEdges: u32(16), ReverseOffsets: u64(17), ReverseNeighbors: u32(18), ReverseEdges: u32(19), LabelIndex: graphdata.LabelEntries{Bytes: sections[19]}, LabelPostings: u32(21), IndexedKeys: u32(22), PropertyIndex: graphdata.PropertyEntries{Bytes: sections[22]}, PropertyPostings: u32(24), Partial: le.Uint64(b[16:]) == 1}
	g, err := graph.NewContext(ctx, d, back.Close)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrCorruptSnapshot, err)
	}
	return g, nil
}
