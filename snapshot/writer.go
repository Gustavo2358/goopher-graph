package snapshot

import (
	"bufio"
	"context"
	"errors"
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"gophergraph/snapshot/ports"
	"io"
	"math"
)

type section struct {
	offset, size, count uint64
	u32                 graphdata.U32
	u64                 graphdata.U64
	props               graphdata.Properties
}

func layout(ctx context.Context, d *graphdata.Data) ([24]section, uint64, error) {
	var s [24]section
	s[0].count = d.StringCount() + 1
	var totalStrings uint64
	if d.Strings != nil {
		for i, str := range d.Strings {
			if i%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return s, 0, e
				}
			}
			if uint64(len(str)) > math.MaxInt64-totalStrings {
				return s, 0, errors.New("snapshot exceeds platform size")
			}
			totalStrings += uint64(len(str))
		}
	} else {
		totalStrings = uint64(len(d.StringBytes))
	}
	s[1].count = totalStrings
	for i, c := range map[int]graphdata.U32{2: d.NodeIDs, 4: d.NodeLabels, 7: d.EdgeIDs, 8: d.Sources, 9: d.Targets, 10: d.EdgeLabels, 14: d.ForwardNeighbors, 15: d.ForwardEdges, 17: d.ReverseNeighbors, 18: d.ReverseEdges, 20: d.LabelPostings, 21: d.IndexedKeys, 23: d.PropertyPostings} {
		s[i].u32 = c
		s[i].count = c.Len()
	}
	for i, c := range map[int]graphdata.U64{3: d.NodeLabelOffsets, 5: d.NodePropOffsets, 11: d.EdgePropOffsets, 13: d.ForwardOffsets, 16: d.ReverseOffsets} {
		s[i].u64 = c
		s[i].count = c.Len()
	}
	s[6].props = d.NodeProps
	s[6].count = d.NodeProps.Len()
	s[12].props = d.EdgeProps
	s[12].count = d.EdgeProps.Len()
	s[19].count = d.LabelIndex.Len()
	s[22].count = d.PropertyIndex.Len()
	next := uint64(832)
	for i := range s {
		if s[i].count > (math.MaxInt64-next-63)/strides[i] {
			return s, 0, errors.New("snapshot size overflow")
		}
		s[i].offset = next
		s[i].size = s[i].count * strides[i]
		next = (next + s[i].size + 63) &^ 63
	}
	return s, next, nil
}

// fullWriter accepts legitimate short writes and rejects lack of progress.
type fullWriter struct{ io.Writer }

func (w fullWriter) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n, e := w.Writer.Write(b)
		if n < 0 || n > len(b) {
			return total, io.ErrShortWrite
		}
		total += n
		b = b[n:]
		if e != nil {
			return total, e
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
func Write(ctx context.Context, g *graph.Graph, sink ports.Sink) (pub ports.Publication, err error) {
	if e := ctx.Err(); e != nil {
		return pub, e
	}
	if sink == nil {
		return pub, errors.New("nil snapshot sink")
	}
	var d graphdata.Data
	if e := g.InternalColumns(&d); e != nil {
		return pub, e
	}
	sections, size, e := layout(ctx, &d)
	if e != nil {
		return pub, e
	}
	tx, e := sink.Begin(ctx, size)
	if e != nil {
		return pub, e
	}
	if tx == nil {
		return pub, errors.New("sink returned nil transaction")
	}
	defer func() { err = errors.Join(err, tx.Abort()) }()
	w := bufio.NewWriterSize(fullWriter{tx}, 65536)
	header := make([]byte, 832)
	copy(header, "GOPHGRPH")
	le.PutUint32(header[8:], 1)
	le.PutUint32(header[12:], 64)
	if d.Partial {
		le.PutUint64(header[16:], 1)
	}
	le.PutUint64(header[24:], size)
	le.PutUint64(header[32:], d.NodeIDs.Len())
	le.PutUint64(header[40:], d.EdgeIDs.Len())
	le.PutUint32(header[48:], uint32(d.StringCount()))
	le.PutUint32(header[52:], 24)
	le.PutUint64(header[56:], 64)
	for i, s := range sections {
		entry := header[64+i*32:]
		le.PutUint32(entry, uint32(i+1))
		le.PutUint32(entry[4:], uint32(strides[i]))
		le.PutUint64(entry[8:], s.offset)
		le.PutUint64(entry[16:], s.size)
		le.PutUint64(entry[24:], s.count)
	}
	if _, e = w.Write(header); e != nil {
		return pub, e
	}
	var zero [64]byte
	for i, s := range sections {
		if e = writeSection(ctx, w, &d, i, s); e != nil {
			return pub, e
		}
		end := s.offset + s.size
		pad := ((end + 63) &^ 63) - end
		if _, e = w.Write(zero[:pad]); e != nil {
			return pub, e
		}
	}
	if e = w.Flush(); e != nil {
		return pub, e
	}
	if e = ctx.Err(); e != nil {
		return pub, e
	}
	back, e := tx.Seal(ctx)
	if e != nil {
		return pub, e
	}
	if back == nil {
		return pub, errors.New("seal returned nil backing")
	}
	view, e := openBacking(ctx, back)
	if e != nil {
		return pub, errors.Join(e, back.Close())
	}
	if e = view.Close(); e != nil {
		return pub, e
	}
	if e = ctx.Err(); e != nil {
		return pub, e
	}
	return tx.Commit(ctx)
}
func writeSection(ctx context.Context, w *bufio.Writer, d *graphdata.Data, index int, s section) error {
	if index == 1 {
		if d.Strings == nil {
			return writeBytes(ctx, w, d.StringBytes)
		}
		for _, str := range d.Strings {
			for len(str) > 0 {
				if e := ctx.Err(); e != nil {
					return e
				}
				n := min(len(str), 65536)
				if _, e := w.WriteString(str[:n]); e != nil {
					return e
				}
				str = str[n:]
			}
		}
		return nil
	}
	var record [32]byte
	var stringOffset uint64
	for i := uint64(0); i < s.count; i++ {
		if i%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return e
			}
		}
		clear(record[:])
		switch index {
		case 0:
			if d.Strings == nil {
				le.PutUint64(record[:], d.StringOffsets.At(i))
			} else {
				le.PutUint64(record[:], stringOffset)
				if i < d.StringCount() {
					stringOffset += uint64(len(d.Strings[i]))
				}
			}
		case 6, 12:
			p := s.props.At(i)
			le.PutUint32(record[:], p.Key)
			record[4] = p.Kind
			le.PutUint64(record[8:], p.Payload)
		case 19:
			p := d.LabelIndex.At(i)
			le.PutUint32(record[:], p.Owner)
			le.PutUint32(record[4:], p.Label)
			le.PutUint64(record[8:], p.Start)
			le.PutUint64(record[16:], p.Count)
		case 22:
			p := d.PropertyIndex.At(i)
			record[0] = p.Owner
			record[1] = p.Kind
			le.PutUint32(record[4:], p.Key)
			le.PutUint64(record[8:], p.Payload)
			le.PutUint64(record[16:], p.Start)
			le.PutUint64(record[24:], p.Count)
		default:
			if strides[index] == 4 {
				le.PutUint32(record[:], s.u32.At(i))
			} else {
				le.PutUint64(record[:], s.u64.At(i))
			}
		}
		if _, e := w.Write(record[:strides[index]]); e != nil {
			return e
		}
	}
	return nil
}
func writeBytes(ctx context.Context, w io.Writer, b []byte) error {
	for len(b) > 0 {
		if e := ctx.Err(); e != nil {
			return e
		}
		n := min(len(b), 65536)
		if _, e := w.Write(b[:n]); e != nil {
			return e
		}
		b = b[n:]
	}
	return nil
}
