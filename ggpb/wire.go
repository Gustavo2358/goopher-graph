package ggpb

import (
	"context"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/query"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// EmitEncoded emits canonical Protobuf Batch payloads, without file framing.
// The bytes are borrowed until send returns; retaining/asynchronous transports
// must copy them. Emit provides generated messages for ordinary typed transports.
// No encoder memory or graph storage may be mutated by send.
func EmitEncoded(ctx context.Context, g *graph.Graph, s *query.Subgraph, q Query, o Options, send func([]byte) error) error {
	var w wireEncoder
	return Emit(ctx, g, s, q, o, func(b *pb.Batch) error {
		payload := w.batch(b)
		if len(payload) > MaxFrame {
			return ErrLimit
		}
		return send(payload)
	})
}

// Official protowire primitives; result.proto remains the contract. This codec
// handles encoder-created messages only (no unknown fields). Differential tests
// compare every emitted payload against the generated deterministic marshaler.
type wireEncoder struct{ out, records, part []byte }

func field(dst []byte, n protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(dst, n, protowire.VarintType), v)
}
func str(dst []byte, n protowire.Number, s string) []byte {
	return protowire.AppendString(protowire.AppendTag(dst, n, protowire.BytesType), s)
}
func msg(dst []byte, n protowire.Number, b []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(dst, n, protowire.BytesType), b)
}
func symSize(s *pb.Symbol) int {
	n := 0
	if s.Ref != 0 {
		n = 1 + protowire.SizeVarint(uint64(s.Ref))
	}
	if s.Text != "" {
		n += 1 + protowire.SizeBytes(len(s.Text))
	}
	return n
}
func sym(dst []byte, n protowire.Number, s *pb.Symbol) []byte {
	dst = protowire.AppendTag(dst, n, protowire.BytesType)
	dst = protowire.AppendVarint(dst, uint64(symSize(s)))
	if s.Ref != 0 {
		dst = field(dst, 1, uint64(s.Ref))
	}
	if s.Text != "" {
		dst = str(dst, 2, s.Text)
	}
	return dst
}
func prop(dst []byte, n protowire.Number, p *pb.Property) []byte {
	size := 1 + protowire.SizeBytes(symSize(p.Key)) + 2
	switch v := p.Value.(type) {
	case *pb.Property_Boolean:
		size += 2
		_ = v
	case *pb.Property_Integer:
		size += 1 + protowire.SizeVarint(protowire.EncodeZigZag(v.Integer))
	case *pb.Property_FloatValue:
		size += 5
	case *pb.Property_DoubleValue:
		size += 9
	case *pb.Property_Text:
		size += 1 + protowire.SizeBytes(len(v.Text))
	}
	dst = protowire.AppendTag(dst, n, protowire.BytesType)
	dst = protowire.AppendVarint(dst, uint64(size))
	dst = sym(dst, 1, p.Key)
	dst = field(dst, 2, uint64(p.Kind))
	switch v := p.Value.(type) {
	case *pb.Property_Boolean:
		x := uint64(0)
		if v.Boolean {
			x = 1
		}
		dst = field(dst, 3, x)
	case *pb.Property_Integer:
		dst = field(dst, 4, protowire.EncodeZigZag(v.Integer))
	case *pb.Property_FloatValue:
		dst = protowire.AppendFixed32(protowire.AppendTag(dst, 5, protowire.Fixed32Type), math.Float32bits(v.FloatValue))
	case *pb.Property_DoubleValue:
		dst = protowire.AppendFixed64(protowire.AppendTag(dst, 6, protowire.Fixed64Type), math.Float64bits(v.DoubleValue))
	case *pb.Property_Text:
		dst = str(dst, 7, v.Text)
	}
	return dst
}
func (w *wireEncoder) batch(b *pb.Batch) []byte {
	w.out = w.out[:0]
	if h := b.GetHeader(); h != nil {
		r := w.records[:0]
		r = field(r, 1, uint64(h.Version))
		q := w.part[:0]
		if h.Query.Name != "" {
			q = str(q, 1, h.Query.Name)
		}
		for i, s := range []*string{h.Query.Node, h.Query.From, h.Query.To} {
			if s != nil {
				q = str(q, protowire.Number(i+2), *s)
			}
		}
		if h.Query.Filtered {
			q = field(q, 5, 1)
		}
		for _, s := range h.Query.EdgeLabels {
			q = str(q, 6, s)
		}
		r = msg(r, 2, q)
		if h.Directed {
			r = field(r, 3, 1)
		}
		if h.PartialSnapshot {
			r = field(r, 4, 1)
		}
		if h.Nodes != 0 {
			r = field(r, 5, h.Nodes)
		}
		if h.Edges != 0 {
			r = field(r, 6, h.Edges)
		}
		w.part = q
		w.records = r
		w.out = msg(w.out, 1, r)
	} else if r := b.GetRecords(); r != nil {
		buf := w.records[:0]
		for _, s := range r.Dictionary {
			buf = str(buf, 1, s)
		}
		for _, p := range r.Parts {
			part := w.part[:0]
			num := protowire.Number(1)
			if n := p.GetNode(); n != nil {
				if n.Id != nil {
					part = str(part, 1, *n.Id)
				}
				for _, s := range n.Labels {
					part = sym(part, 2, s)
				}
				for _, v := range n.Properties {
					part = prop(part, 3, v)
				}
				if n.Last {
					part = field(part, 4, 1)
				}
			} else if n := p.GetEdge(); n != nil {
				num = 2
				if n.Id != nil {
					part = str(part, 1, *n.Id)
				}
				if n.Source != nil {
					part = str(part, 2, *n.Source)
				}
				if n.Target != nil {
					part = str(part, 3, *n.Target)
				}
				if n.Label != nil {
					part = sym(part, 4, n.Label)
				}
				for _, v := range n.Properties {
					part = prop(part, 5, v)
				}
				if n.Last {
					part = field(part, 6, 1)
				}
				if n.SourceRef != 0 {
					part = field(part, 7, uint64(n.SourceRef))
				}
				if n.TargetRef != 0 {
					part = field(part, 8, uint64(n.TargetRef))
				}
			}
			// Records.parts -> Part.entity -> NodePart/EdgePart.
			size := 1 + protowire.SizeBytes(len(part))
			buf = protowire.AppendTag(buf, 2, protowire.BytesType)
			buf = protowire.AppendVarint(buf, uint64(size))
			buf = msg(buf, num, part)
			w.part = part
		}
		for _, s := range r.EndpointIds {
			buf = str(buf, 3, s)
		}
		w.records = buf
		w.out = msg(w.out, 2, buf)
	} else if e := b.GetEnd(); e != nil {
		r := w.records[:0]
		if e.Nodes != 0 {
			r = field(r, 1, e.Nodes)
		}
		if e.Edges != 0 {
			r = field(r, 2, e.Edges)
		}
		w.records = r
		w.out = msg(w.out, 3, r)
	}
	return w.out
}
