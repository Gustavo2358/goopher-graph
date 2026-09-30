package graphdata

import (
	"context"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid graph columns")

func invalid(s string) error { return fmt.Errorf("%w: %s", ErrInvalid, s) }
func offsets(ctx context.Context, c U64, owners, total uint64) bool {
	if c.Len() != owners+1 || c.At(0) != 0 || c.At(owners) != total {
		return false
	}
	for i := uint64(1); i < c.Len(); i++ {
		if i%1024 == 0 && ctx.Err() != nil {
			return false
		}
		if c.At(i) < c.At(i-1) || c.At(i) > total {
			return false
		}
	}
	return true
}
func LessProperty(a, b Property) bool {
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Payload < b.Payload
}
func ValidPayload(p Property, strings uint64) bool {
	switch p.Kind {
	case 1:
		return p.Payload <= 1
	case 2:
		return uint64(int64(int8(p.Payload))) == p.Payload
	case 3:
		return uint64(int64(int16(p.Payload))) == p.Payload
	case 4:
		return uint64(int64(int32(p.Payload))) == p.Payload
	case 5:
		return true
	case 6:
		return p.Payload <= math.MaxUint32 && (!math.IsNaN(float64(math.Float32frombits(uint32(p.Payload)))) || p.Payload == 0x7fc00000)
	case 7:
		return !math.IsNaN(math.Float64frombits(p.Payload)) || p.Payload == 0x7ff8000000000000
	case 8, 9, 10:
		return p.Payload < strings
	}
	return false
}
func Validate(d *Data) error { return ValidateContext(context.Background(), d) }
func ValidateContext(ctx context.Context, d *Data) (err error) {
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	if e := ctx.Err(); e != nil {
		return e
	}
	if d == nil {
		return invalid("nil")
	}
	n, e, s := d.NodeIDs.Len(), d.EdgeIDs.Len(), d.StringCount()
	if n > math.MaxUint32 || e > math.MaxUint32 || s == 0 || s > math.MaxUint32 {
		return invalid("counts")
	}
	if d.Strings == nil && !offsets(ctx, d.StringOffsets, s, uint64(len(d.StringBytes))) {
		return invalid("string offsets")
	}
	for i := uint64(0); i < s; i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		v := d.String(uint32(i))
		if !utf8.ValidString(v) || (i == 0 && v != "") || (i > 0 && d.String(uint32(i-1)) >= v) {
			return invalid("dictionary")
		}
	}
	for _, c := range []U32{d.NodeIDs, d.EdgeIDs} {
		for i := uint64(0); i < c.Len(); i++ {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if uint64(c.At(i)) >= s || (i > 0 && c.At(i-1) >= c.At(i)) {
				return invalid("external ids")
			}
		}
	}
	if !offsets(ctx, d.NodeLabelOffsets, n, d.NodeLabels.Len()) {
		return invalid("labels offsets")
	}
	for u := uint64(0); u < n; u++ {
		if u%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		a, b := d.NodeLabelOffsets.At(u), d.NodeLabelOffsets.At(u+1)
		if a == b {
			return invalid("missing node labels")
		}
		for j := a; j < b; j++ {
			if j%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			v := d.NodeLabels.At(j)
			if v == 0 || uint64(v) >= s || (j > a && d.NodeLabels.At(j-1) >= v) {
				return invalid("node labels")
			}
		}
	}
	if d.Sources.Len() != e || d.Targets.Len() != e || d.EdgeLabels.Len() != e {
		return invalid("edge columns")
	}
	for i := uint64(0); i < e; i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if uint64(d.Sources.At(i)) >= n || uint64(d.Targets.At(i)) >= n || d.EdgeLabels.At(i) == 0 || uint64(d.EdgeLabels.At(i)) >= s {
			return invalid("edge reference")
		}
	}
	for owner, v := range []struct {
		o     U64
		p     Properties
		count uint64
	}{{d.NodePropOffsets, d.NodeProps, n}, {d.EdgePropOffsets, d.EdgeProps, e}} {
		if !offsets(ctx, v.o, v.count, v.p.Len()) {
			return invalid("property offsets")
		}
		for u := uint64(0); u < v.count; u++ {
			if u%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			a, b := v.o.At(u), v.o.At(u+1)
			for j := a; j < b; j++ {
				if j%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				p := v.p.At(j)
				if p.Key == 0 || uint64(p.Key) >= s || !ValidPayload(p, s) {
					return invalid("property")
				}
				if j > a {
					prev := v.p.At(j - 1)
					if !LessProperty(prev, p) || (owner == 1 && prev.Key == p.Key) {
						return invalid("property ordering")
					}
				}
			}
		}
	}
	// Membership fixes each edge's owner/neighbor/label. Strict tuple ordering
	// then forbids repeats within that owner; the total count proves coverage.
	// No graph-sized seen bitset is needed (including during mmap validation).
	for dir, v := range []struct {
		o                U64
		neighbors, edges U32
	}{{d.ForwardOffsets, d.ForwardNeighbors, d.ForwardEdges}, {d.ReverseOffsets, d.ReverseNeighbors, d.ReverseEdges}} {
		if !offsets(ctx, v.o, n, e) || v.neighbors.Len() != e || v.edges.Len() != e {
			return invalid("CSR shape")
		}
		for u := uint64(0); u < n; u++ {
			if u%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			a, b := v.o.At(u), v.o.At(u+1)
			var prevNeighbor, prevLabel, prevEdge uint32
			for j := a; j < b; j++ {
				if j%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				id, neighbor := v.edges.At(j), v.neighbors.At(j)
				if uint64(id) >= e || uint64(neighbor) >= n {
					return invalid("CSR id")
				}
				src, dst := d.Sources.At(uint64(id)), d.Targets.At(uint64(id))
				if dir == 1 {
					src, dst = dst, src
				}
				label := d.EdgeLabels.At(uint64(id))
				if uint64(src) != u || dst != neighbor {
					return invalid("CSR membership")
				}
				if j > a && (neighbor < prevNeighbor || (neighbor == prevNeighbor && (label < prevLabel || (label == prevLabel && id <= prevEdge)))) {
					return invalid("CSR order")
				}
				prevNeighbor, prevLabel, prevEdge = neighbor, label, id
			}
		}
	}
	return validateIndexes(ctx, d)
}
