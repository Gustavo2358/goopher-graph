package ggpb

import (
	"context"
	"fmt"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"unicode/utf8"
)

type validator struct {
	open                   int // 0 closed, 1 node, 2 edge
	edges, properties      bool
	nodesCount, edgesCount uint64
	lastNode, lastEdge     string
	haveNode, haveEdge     bool
}

func invalid(why string) error { return fmt.Errorf("%w: %s", ErrInvalid, why) }
func text(s string) error {
	if len(s) > MaxScalar {
		return ErrLimit
	}
	if !utf8.ValidString(s) {
		return invalid("UTF-8")
	}
	return nil
}
func (v *validator) header(h *pb.ResultHeader) error {
	if !h.Directed || h.Query == nil {
		return invalid("header metadata")
	}
	q := h.Query
	for _, s := range []*string{&q.Name, q.Node, q.From, q.To} {
		if s != nil {
			if e := text(*s); e != nil {
				return e
			}
		}
	}
	if !q.Filtered && len(q.EdgeLabels) > 0 {
		return invalid("unrestricted filter with labels")
	}
	for i, s := range q.EdgeLabels {
		if e := text(s); e != nil {
			return e
		}
		if i > 0 && q.EdgeLabels[i-1] >= s {
			return invalid("filter order")
		}
	}
	return nil
}
func symbol(s *pb.Symbol, d []string) (string, error) {
	if s == nil {
		return "", invalid("missing symbol")
	}
	if s.Ref == 0 {
		return s.Text, text(s.Text)
	}
	if uint64(s.Ref) > uint64(len(d)) || s.Text != "" {
		return "", invalid("dictionary reference")
	}
	return d[s.Ref-1], nil
}
func value(p *pb.Property) (graph.Value, error) {
	k := graph.ValueKind(p.Kind)
	switch x := p.Value.(type) {
	case *pb.Property_Boolean:
		if k == graph.BoolKind {
			return graph.BoolValue(x.Boolean), nil
		}
	case *pb.Property_Integer:
		return graph.IntegerValue(k, x.Integer)
	case *pb.Property_FloatValue:
		if k == graph.FloatKind {
			return graph.DecimalValue(k, float64(x.FloatValue))
		}
	case *pb.Property_DoubleValue:
		if k == graph.DoubleKind {
			return graph.DecimalValue(k, x.DoubleValue)
		}
	case *pb.Property_Text:
		if e := text(x.Text); e != nil {
			return graph.Value{}, e
		}
		return graph.TextValue(k, x.Text)
	}
	return graph.Value{}, invalid("property type/value")
}
func (v *validator) records(ctx context.Context, r *pb.Records) error {
	if r == nil || len(r.Parts) == 0 || len(r.Parts) > maxParts || len(r.Dictionary) > maxDictionary {
		return invalid("batch bounds")
	}
	size := 0
	for _, s := range r.Dictionary {
		if e := text(s); e != nil {
			return e
		}
		size += len(s)
		if size > dictionaryBytes {
			return ErrLimit
		}
	}
	if len(r.EndpointIds) > maxEndpoints {
		return invalid("endpoint bounds")
	}
	endpointSize := 0
	for _, s := range r.EndpointIds {
		if e := text(s); e != nil {
			return e
		}
		endpointSize += len(s)
		if endpointSize > endpointBytes {
			return ErrLimit
		}
	}
	props := func(ps []*pb.Property) error {
		for _, p := range ps {
			if e := ctx.Err(); e != nil {
				return e
			}
			if p == nil {
				return invalid("missing property")
			}
			if _, e := symbol(p.Key, r.Dictionary); e != nil {
				return e
			}
			if _, e := value(p); e != nil {
				return fmt.Errorf("%w: property: %w", ErrInvalid, e)
			}
		}
		return nil
	}
	for _, p := range r.Parts {
		if e := ctx.Err(); e != nil {
			return e
		}
		if p == nil {
			return invalid("missing part")
		}
		switch x := p.Entity.(type) {
		case *pb.Part_Node:
			n := x.Node
			if n == nil || v.edges {
				return invalid("node after edges")
			}
			if n.Id != nil {
				if v.open != 0 {
					return invalid("interleaved entity")
				}
				if e := text(*n.Id); e != nil {
					return e
				}
				if v.haveNode && v.lastNode >= *n.Id {
					return invalid("node ID order")
				}
				v.haveNode = true
				v.lastNode = *n.Id
				v.open = 1
				v.properties = false
			} else if v.open != 1 {
				return invalid("orphan node continuation")
			}
			if v.properties && len(n.Labels) > 0 {
				return invalid("label after property")
			}
			for _, s := range n.Labels {
				if _, e := symbol(s, r.Dictionary); e != nil {
					return e
				}
			}
			if len(n.Properties) > 0 {
				v.properties = true
			}
			if e := props(n.Properties); e != nil {
				return e
			}
			if n.Last {
				v.open = 0
				v.nodesCount++
			}
		case *pb.Part_Edge:
			n := x.Edge
			if n == nil {
				return invalid("missing edge")
			}
			if n.Id != nil {
				if v.open != 0 || n.Label == nil {
					return invalid("edge start")
				}
				if _, e := endpoint(n.Source, n.SourceRef, r.EndpointIds); e != nil {
					return e
				}
				if _, e := endpoint(n.Target, n.TargetRef, r.EndpointIds); e != nil {
					return e
				}
				for _, s := range []string{*n.Id} {
					if e := text(s); e != nil {
						return e
					}
				}
				if v.haveEdge && v.lastEdge >= *n.Id {
					return invalid("edge ID order")
				}
				if _, e := symbol(n.Label, r.Dictionary); e != nil {
					return e
				}
				v.haveEdge = true
				v.lastEdge = *n.Id
				v.open = 2
				v.edges = true
			} else if v.open != 2 || n.Source != nil || n.Target != nil || n.SourceRef != 0 || n.TargetRef != 0 || n.Label != nil {
				return invalid("edge continuation")
			}
			if e := props(n.Properties); e != nil {
				return e
			}
			if n.Last {
				v.open = 0
				v.edgesCount++
			}
		default:
			return invalid("unknown part")
		}
	}
	return nil
}
func (v *validator) end(e *pb.ResultEnd, nodes, edges uint64) error {
	if e == nil || v.open != 0 || e.Nodes != v.nodesCount || e.Edges != v.edgesCount || nodes != e.Nodes || edges != e.Edges {
		return invalid("incomplete result or counts")
	}
	return nil
}

func endpoint(inline *string, ref uint32, ids []string) (string, error) {
	if ref != 0 {
		if inline != nil || uint64(ref) > uint64(len(ids)) {
			return "", invalid("endpoint reference")
		}
		return ids[ref-1], nil
	}
	if inline == nil {
		return "", invalid("missing endpoint")
	}
	return *inline, text(*inline)
}
