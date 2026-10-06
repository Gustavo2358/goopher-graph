// Package ggpb streams logical, self-contained Protobuf query results.
// It uses only public graph/query APIs, never snapshot columns or bitmaps.
package ggpb

import (
	"context"
	"errors"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/query"
	"slices"

	"google.golang.org/protobuf/proto"
)

const Version = 1
const MaxFrame = 4 << 20
const MaxScalar = 1 << 20
const batchTarget = 256 << 10
const partTarget = 32 << 10
const maxParts = 256
const maxDictionary = 1024
const dictionaryBytes = 64 << 10

var ErrLimit = errors.New("ggpb: message or scalar exceeds protocol limit")
var ErrInvalid = errors.New("ggpb: invalid stream")
var ErrVersion = errors.New("ggpb: incompatible protocol version")

type Query struct {
	Name       string   `json:"name"`
	Node       *string  `json:"node,omitempty"`
	From       *string  `json:"from,omitempty"`
	To         *string  `json:"to,omitempty"`
	EdgeLabels []string `json:"edgeLabels"`
}

// Options permits a measured inline-symbol baseline. Both modes use protocol v1.
type Options struct{ InlineSymbols bool }

// Emit emits bounded logical messages synchronously. send owns no graph storage;
// its Batch is borrowed until send returns. A future gRPC adapter can send these
// messages directly. Keep the graph open and inputs unchanged during Emit.
// Memory: a bounded batch/dictionary, one part and bounded metadata.
// Labels and properties use public iterators, with no result-wide DTO.
func Emit(ctx context.Context, g *graph.Graph, s *query.Subgraph, q Query, options Options, send func(*pb.Batch) error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if !s.BelongsTo(g) {
		return graph.ErrGraphMismatch
	}
	if _, _, e := g.FindString(""); e != nil {
		return e
	}
	for _, v := range []*string{&q.Name, q.Node, q.From, q.To} {
		if v != nil && len(*v) > MaxScalar {
			return ErrLimit
		}
	}
	// Check metadata before cloning, including an adversarial filter list.
	size := len(q.Name) + 64
	for _, v := range []*string{q.Node, q.From, q.To} {
		if v != nil {
			size += len(*v) + 8
		}
	}
	for _, v := range q.EdgeLabels {
		if len(v) > MaxScalar || len(v)+8 > MaxFrame-size {
			return ErrLimit
		}
		size += len(v) + 8
	}
	labels := slices.Clone(q.EdgeLabels)
	slices.Sort(labels)
	labels = slices.Compact(labels)
	h := &pb.ResultHeader{Version: Version, Query: &pb.Query{Name: q.Name, Node: q.Node, From: q.From, To: q.To, Filtered: q.EdgeLabels != nil, EdgeLabels: labels}, Directed: true, PartialSnapshot: g.Metadata().PartialLoad, Nodes: s.NodeCount(), Edges: s.EdgeCount()}
	if e := new(validator).header(h); e != nil {
		return e
	}
	emit := func(b *pb.Batch) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if proto.Size(b) > MaxFrame {
			return ErrLimit
		}
		return send(b)
	}
	if e := emit(&pb.Batch{Payload: &pb.Batch_Header{Header: h}}); e != nil {
		return e
	}
	enc := batcher{emit: emit, inline: options.InlineSymbols, g: g}
	ni := s.Nodes()
	for ni.Next() {
		if e := ctx.Err(); e != nil {
			return e
		}
		id, e := g.NodeExternalID(ni.ID())
		if e != nil {
			return e
		}
		if len(id) > MaxScalar {
			return ErrLimit
		}
		slot := enc.slot()
		slot.id = id
		n := &slot.node
		n.Id = &slot.id
		slot.nodeWrap.Node = n
		slot.part.Entity = &slot.nodeWrap
		size := len(id) + 8
		flush := func(last bool) error {
			n.Last = last
			n.Labels = slot.ls
			n.Properties = slot.ps
			e := enc.addSlot(slot, size)
			if !last {
				slot = enc.slot()
				n = &slot.node
				slot.nodeWrap.Node = n
				slot.part.Entity = &slot.nodeWrap
			}
			size = 0
			return e
		}
		ls, e := g.IterateNodeLabels(ni.ID())
		if e != nil {
			return e
		}
		for ls.Next() {
			if e := ctx.Err(); e != nil {
				return e
			}
			text, e := enc.string(ls.ID())
			if e != nil {
				return e
			}
			if len(text) > MaxScalar {
				return ErrLimit
			}
			if size >= partTarget {
				if e := flush(false); e != nil {
					return e
				}
			}
			slot.label(text, uint32(ls.ID())+1)
			size += len(text) + 12
		}
		if e := ls.Err(); e != nil {
			return e
		}
		props, e := g.NodeProperties(ni.ID())
		if e != nil {
			return e
		}
		for props.Next() {
			if e := ctx.Err(); e != nil {
				return e
			}
			if size >= partTarget {
				if e := flush(false); e != nil {
					return e
				}
			}
			p, e := enc.property(slot, props.Property())
			if e != nil {
				return e
			}
			slot.ps = append(slot.ps, p)
			size += propertyBound(p)
		}
		if e := props.Err(); e != nil {
			return e
		}
		if e := flush(true); e != nil {
			return e
		}
	}
	ei := s.Edges()
	for ei.Next() {
		if e := ctx.Err(); e != nil {
			return e
		}
		edge, e := g.Edge(ei.ID())
		if e != nil {
			return e
		}
		id, e := g.EdgeExternalID(ei.ID())
		if e != nil {
			return e
		}
		source, e := enc.endpointText(edge.Source)
		if e != nil {
			return e
		}
		target, e := enc.endpointText(edge.Target)
		if e != nil {
			return e
		}
		label, e := enc.string(edge.Label)
		if e != nil {
			return e
		}
		for _, v := range []string{id, source, target, label} {
			if len(v) > MaxScalar {
				return ErrLimit
			}
		}
		slot := enc.slot()
		slot.id = id
		slot.source = source
		slot.target = target
		n := &slot.edge
		n.Id = &slot.id
		n.Source = &slot.source
		n.Target = &slot.target
		n.SourceRef = uint32(edge.Source) + 1
		n.TargetRef = uint32(edge.Target) + 1
		n.Label = slot.label(label, uint32(edge.Label)+1)
		slot.edgeWrap.Edge = n
		slot.part.Entity = &slot.edgeWrap
		size := len(id) + len(source) + len(target) + len(label) + 64
		flush := func(last bool) error {
			n.Last = last
			n.Properties = slot.ps
			e := enc.addSlot(slot, size)
			if !last {
				slot = enc.slot()
				n = &slot.edge
				slot.edgeWrap.Edge = n
				slot.part.Entity = &slot.edgeWrap
			}
			size = 0
			return e
		}
		props, e := g.EdgeProperties(ei.ID())
		if e != nil {
			return e
		}
		for props.Next() {
			if e := ctx.Err(); e != nil {
				return e
			}
			if size >= partTarget {
				if e := flush(false); e != nil {
					return e
				}
			}
			p, e := enc.property(slot, props.Property())
			if e != nil {
				return e
			}
			slot.ps = append(slot.ps, p)
			size += propertyBound(p)
		}
		if e := props.Err(); e != nil {
			return e
		}
		if e := flush(true); e != nil {
			return e
		}
	}
	if e := enc.flush(); e != nil {
		return e
	}
	return emit(&pb.Batch{Payload: &pb.Batch_End{End: &pb.ResultEnd{Nodes: h.Nodes, Edges: h.Edges}}})
}

type batcher struct {
	emit               func(*pb.Batch) error
	records            *pb.Records
	dict               map[graph.StringID]uint32
	g                  *graph.Graph
	cache              map[graph.StringID]string
	cacheBytes         int
	endpointCache      map[graph.NodeID]string
	endpointCacheBytes int
	endpoints          map[graph.NodeID]uint32
	endpointBytes      int
	free, used         []*recordSlot
	size, textBytes    int
	inline             bool
}

func (b *batcher) flush() error {
	if b.records == nil || len(b.records.Parts) == 0 {
		return nil
	}
	e := b.emit(&pb.Batch{Payload: &pb.Batch_Records{Records: b.records}})
	for _, s := range b.used {
		if cap(s.props) <= 16 && cap(s.labels) <= 16 && len(b.free) < maxParts {
			s.reset()
			b.free = append(b.free, s)
		}
	}
	clear(b.used)
	b.used = b.used[:0]
	b.records = nil
	b.dict = nil
	b.cache = nil
	b.cacheBytes = 0
	b.endpointCache = nil
	b.endpointCacheBytes = 0
	b.endpoints = nil
	b.endpointBytes = 0
	b.size = 0
	b.textBytes = 0
	return e
}
func (b *batcher) symbol(s *pb.Symbol) {
	if s == nil {
		return
	}
	idKey := graph.StringID(s.Ref - 1)
	if b.inline {
		s.Ref = 0
		return
	}
	if id, ok := b.dict[idKey]; ok {
		s.Ref = id
		s.Text = ""
		return
	}
	if len(b.dict) >= maxDictionary || len(s.Text)+b.textBytes > dictionaryBytes {
		s.Ref = 0
		return
	}
	b.textBytes += len(s.Text)
	b.records.Dictionary = append(b.records.Dictionary, s.Text)
	id := uint32(len(b.records.Dictionary))
	b.dict[idKey] = id
	s.Ref = id
	s.Text = ""
}
func (b *batcher) add(p *pb.Part, size int) error {
	// Size before dictionary substitution is an upper bound, including new entries.
	size += 16
	if b.records != nil && (b.size+size > batchTarget || len(b.records.Parts) >= maxParts) {
		if e := b.flush(); e != nil {
			return e
		}
	}
	if b.records == nil {
		b.records = &pb.Records{}
		b.dict = make(map[graph.StringID]uint32)
	}
	if n := p.GetNode(); n != nil {
		for _, s := range n.Labels {
			b.symbol(s)
		}
		for _, p := range n.Properties {
			b.symbol(p.Key)
		}
	}
	if n := p.GetEdge(); n != nil {
		b.endpoint(&n.Source, &n.SourceRef)
		b.endpoint(&n.Target, &n.TargetRef)
		b.symbol(n.Label)
		for _, p := range n.Properties {
			b.symbol(p.Key)
		}
	}
	b.records.Parts = append(b.records.Parts, p)
	b.size += size
	return nil
}
func (b *batcher) property(slot *recordSlot, p graph.Property) (*pb.Property, error) {
	key, e := b.string(p.Key)
	if e != nil {
		return nil, e
	}
	if len(key) > MaxScalar {
		return nil, ErrLimit
	}
	slot.props = append(slot.props, propertySlot{})
	ps := &slot.props[len(slot.props)-1]
	out := &ps.p
	out.Key = &ps.key
	out.Key.Text = key
	out.Key.Ref = uint32(p.Key) + 1
	out.Kind = pb.Kind(p.Value.Kind())
	switch p.Value.Kind() {
	case graph.BoolKind:
		v, _ := p.Value.Bool()
		ps.boolean.Boolean = v
		out.Value = &ps.boolean
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		v, _ := p.Value.Int64()
		ps.integer.Integer = v
		out.Value = &ps.integer
	case graph.FloatKind:
		v, _ := p.Value.Float64()
		ps.float.FloatValue = float32(v)
		out.Value = &ps.float
	case graph.DoubleKind:
		v, _ := p.Value.Float64()
		ps.double.DoubleValue = v
		out.Value = &ps.double
	case graph.StringKind, graph.DateKind, graph.DatetimeKind:
		v, _ := p.Value.Text()
		if len(v) > MaxScalar {
			return nil, ErrLimit
		}
		ps.text.Text = v
		out.Value = &ps.text
	default:
		return nil, graph.ErrInvalidValue
	}
	return out, nil
}

// Conservative wire-size bound; avoids recursive reflection per property.
func propertyBound(p *pb.Property) int {
	n := len(p.Key.Text) + 40
	if x, ok := p.Value.(*pb.Property_Text); ok {
		n += len(x.Text)
	}
	return n
}

// Cache only a bounded batch's labels/keys. StringIDs never reach the callback.
func (b *batcher) string(id graph.StringID) (string, error) {
	if s, ok := b.cache[id]; ok {
		return s, nil
	}
	s, e := b.g.String(id)
	if e != nil {
		return "", e
	}
	if len(s) > MaxScalar {
		return "", ErrLimit
	}
	if !b.inline && len(b.cache) < maxDictionary && b.cacheBytes+len(s) <= dictionaryBytes {
		if b.cache == nil {
			b.cache = make(map[graph.StringID]string)
		}
		b.cache[id] = s
		b.cacheBytes += len(s)
	}
	return s, nil
}

const maxEndpoints = 512
const endpointBytes = 64 << 10

func (b *batcher) endpointText(id graph.NodeID) (string, error) {
	if s, ok := b.endpointCache[id]; ok {
		return s, nil
	}
	s, e := b.g.NodeExternalID(id)
	if e != nil {
		return "", e
	}
	if len(s) > MaxScalar {
		return "", ErrLimit
	}
	if len(b.endpointCache) < maxEndpoints && b.endpointCacheBytes+len(s) <= endpointBytes {
		if b.endpointCache == nil {
			b.endpointCache = make(map[graph.NodeID]string)
		}
		b.endpointCache[id] = s
		b.endpointCacheBytes += len(s)
	}
	return s, nil
}
func (b *batcher) endpoint(inline **string, ref *uint32) {
	if *inline == nil {
		return
	}
	id := graph.NodeID(*ref - 1)
	if v, ok := b.endpoints[id]; ok {
		*ref = v
		*inline = nil
		return
	}
	s := **inline
	if len(b.endpoints) >= maxEndpoints || b.endpointBytes+len(s) > endpointBytes {
		*ref = 0
		return
	}
	if b.endpoints == nil {
		b.endpoints = make(map[graph.NodeID]uint32)
	}
	b.records.EndpointIds = append(b.records.EndpointIds, s)
	b.endpointBytes += len(s)
	*ref = uint32(len(b.records.EndpointIds))
	b.endpoints[id] = *ref
	*inline = nil
}
