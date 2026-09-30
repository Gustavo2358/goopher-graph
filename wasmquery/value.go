package wasmquery

import (
	"context"
	"gophergraph/graph"
	"gophergraph/wasmquery/internal/abi"
	"math"
)

func decodeValue(v abi.Value) (graph.Value, error) {
	k := graph.ValueKind(v.Kind)
	if k < graph.StringKind && v.Text != "" {
		return graph.Value{}, graph.ErrInvalidValue
	}
	switch k {
	case graph.BoolKind:
		if v.Bits > 1 {
			return graph.Value{}, graph.ErrInvalidValue
		}
		return graph.BoolValue(v.Bits == 1), nil
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		return graph.IntegerValue(k, int64(v.Bits))
	case graph.FloatKind:
		if v.Bits > math.MaxUint32 {
			return graph.Value{}, graph.ErrInvalidValue
		}
		return graph.DecimalValue(k, float64(math.Float32frombits(uint32(v.Bits))))
	case graph.DoubleKind:
		return graph.DecimalValue(k, math.Float64frombits(v.Bits))
	case graph.StringKind, graph.DateKind, graph.DatetimeKind:
		if v.Bits != 0 {
			return graph.Value{}, graph.ErrInvalidValue
		}
		return graph.TextValue(k, v.Text)
	}
	return graph.Value{}, graph.ErrInvalidValue
}
func encodeValue(v graph.Value) abi.Value {
	out := abi.Value{Kind: uint8(v.Kind())}
	switch v.Kind() {
	case graph.BoolKind:
		if b, _ := v.Bool(); b {
			out.Bits = 1
		}
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		n, _ := v.Int64()
		out.Bits = uint64(n)
	case graph.FloatKind:
		f, _ := v.Float64()
		out.Bits = uint64(math.Float32bits(float32(f)))
	case graph.DoubleKind:
		f, _ := v.Float64()
		out.Bits = math.Float64bits(f)
	default:
		out.Text, _ = v.Text()
	}
	return out
}
func (s *execution) readProperty(ctx context.Context, edge bool, p abi.Params) (abi.PropertyResult, error) {
	var it graph.PropertyIterator
	if edge {
		id, err := s.g.FindEdge(p.ID)
		if err != nil {
			return abi.PropertyResult{}, err
		}
		it, err = s.g.EdgeProperties(id)
		if err != nil {
			return abi.PropertyResult{}, err
		}
	} else {
		id, err := s.g.FindNode(p.ID)
		if err != nil {
			return abi.PropertyResult{}, err
		}
		it, err = s.g.NodeProperties(id)
		if err != nil {
			return abi.PropertyResult{}, err
		}
	}
	key, found, err := s.g.FindString(p.Key)
	if err != nil || !found {
		return abi.PropertyResult{}, err
	}
	var index uint32
	for it.Next() {
		if err = ctx.Err(); err != nil {
			return abi.PropertyResult{}, err
		}
		prop := it.Property()
		if prop.Key == key {
			if index == p.Index {
				return abi.PropertyResult{Found: true, Value: encodeValue(prop.Value)}, nil
			}
			index++
		}
	}
	return abi.PropertyResult{}, it.Err()
}
