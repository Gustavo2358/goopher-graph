package ggpb

import (
	"bufio"
	"context"
	"encoding/json"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"io"
	"math"
	"strconv"
)

// JSON converts a sequential GGPB stream to the existing graphjson contract
// without a snapshot or materialized result. Failure leaves a disposable prefix.
func JSON(ctx context.Context, out io.Writer, in io.Reader) error {
	r := NewReader(in)
	w := jsonWriter{out: bufio.NewWriter(out), ctx: ctx}
	b, e := r.Next(ctx)
	if e != nil {
		return e
	}
	h := b.GetHeader()
	q := h.Query
	meta := Query{Name: q.Name, Node: q.Node, From: q.From, To: q.To}
	if q.Filtered {
		meta.EdgeLabels = append([]string{}, q.EdgeLabels...)
	}
	w.raw(`{"query":`)
	w.json(meta)
	w.raw(`,"directed":true,"partialSnapshot":`)
	w.raw(strconv.FormatBool(h.PartialSnapshot))
	w.raw(`,"counts":{"nodes":`)
	w.raw(strconv.FormatUint(h.Nodes, 10))
	w.raw(`,"edges":`)
	w.raw(strconv.FormatUint(h.Edges, 10))
	w.raw(`},"nodes":[`)
	nodeSep, edgeSep, labelSep, propSep := "", "", "", ""
	inEdges, inProps := false, false
	for w.err == nil {
		b, e = r.Next(ctx)
		if e != nil {
			if e == io.EOF {
				break
			}
			return e
		}
		if rec := b.GetRecords(); rec != nil {
			sym := func(s *pb.Symbol) string {
				if s.Ref != 0 {
					return rec.Dictionary[s.Ref-1]
				}
				return s.Text
			}
			for _, p := range rec.Parts {
				if w.err != nil {
					break
				}
				if n := p.GetNode(); n != nil {
					if n.Id != nil {
						w.raw(nodeSep)
						nodeSep = ","
						w.raw(`{"id":`)
						w.json(*n.Id)
						w.raw(`,"labels":[`)
						labelSep = ""
						propSep = ""
						inProps = false
					}
					for _, l := range n.Labels {
						w.raw(labelSep)
						labelSep = ","
						w.json(sym(l))
					}
					if len(n.Properties) > 0 || n.Last {
						if !inProps {
							w.raw(`],"properties":[`)
							inProps = true
						}
					}
					for _, p := range n.Properties {
						w.raw(propSep)
						propSep = ","
						w.property(p, sym(p.Key))
					}
					if n.Last {
						w.raw(`]}`)
					}
				} else if n := p.GetEdge(); n != nil {
					if !inEdges {
						w.raw(`],"edges":[`)
						inEdges = true
					}
					if n.Id != nil {
						w.raw(edgeSep)
						edgeSep = ","
						w.raw(`{"id":`)
						w.json(*n.Id)
						w.raw(`,"source":`)
						source, _ := endpoint(n.Source, n.SourceRef, rec.EndpointIds)
						w.json(source)
						w.raw(`,"target":`)
						target, _ := endpoint(n.Target, n.TargetRef, rec.EndpointIds)
						w.json(target)
						w.raw(`,"label":`)
						w.json(sym(n.Label))
						w.raw(`,"properties":[`)
						propSep = ""
					}
					for _, p := range n.Properties {
						w.raw(propSep)
						propSep = ","
						w.property(p, sym(p.Key))
					}
					if n.Last {
						w.raw(`]}`)
					}
				}
			}
		}
	}
	if !inEdges {
		w.raw(`],"edges":[`)
	}
	w.raw("]}\n")
	if w.err != nil {
		return w.err
	}
	return w.out.Flush()
}

type jsonWriter struct {
	out *bufio.Writer
	ctx context.Context
	err error
}

func (w *jsonWriter) raw(s string) {
	if w.err != nil {
		return
	}
	w.err = w.ctx.Err()
	if w.err == nil {
		_, w.err = w.out.WriteString(s)
	}
}
func (w *jsonWriter) json(v any) {
	if w.err != nil {
		return
	}
	b, e := json.Marshal(v)
	if e != nil {
		w.err = e
		return
	}
	w.raw(string(b))
}
func (w *jsonWriter) property(p *pb.Property, key string) {
	names := [...]string{"", "Bool", "Byte", "Short", "Int", "Long", "Float", "Double", "String", "Date", "Datetime"}
	w.raw(`{"key":`)
	w.json(key)
	w.raw(`,"type":`)
	w.json(names[p.Kind])
	w.raw(`,"value":`)
	v, _ := value(p)
	switch v.Kind() {
	case graph.BoolKind:
		b, _ := v.Bool()
		w.raw(strconv.FormatBool(b))
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		n, _ := v.Int64()
		s := strconv.FormatInt(n, 10)
		if v.Kind() == graph.LongKind {
			w.json(s)
		} else {
			w.raw(s)
		}
	case graph.FloatKind, graph.DoubleKind:
		f, _ := v.Float64()
		switch {
		case math.IsNaN(f):
			w.raw(`"NaN"`)
		case math.IsInf(f, 1):
			w.raw(`"+Inf"`)
		case math.IsInf(f, -1):
			w.raw(`"-Inf"`)
		default:
			bits := 64
			if v.Kind() == graph.FloatKind {
				bits = 32
			}
			w.raw(strconv.FormatFloat(f, 'g', -1, bits))
		}
	default:
		s, _ := v.Text()
		w.json(s)
	}
	w.raw(`}`)
}
