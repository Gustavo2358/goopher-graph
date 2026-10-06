package ggpb

import "gophergraph/ggpb/pb"

// Per-encoder reuse only. A slot is recycled after the synchronous callback.
// Large continuation slots are released rather than retained in the cache.
type recordSlot struct {
	part               pb.Part
	nodeWrap           pb.Part_Node
	edgeWrap           pb.Part_Edge
	node               pb.NodePart
	edge               pb.EdgePart
	id, source, target string
	props              []propertySlot
	labels             []pb.Symbol
	ps                 []*pb.Property
	ls                 []*pb.Symbol
}
type propertySlot struct {
	p       pb.Property
	key     pb.Symbol
	boolean pb.Property_Boolean
	integer pb.Property_Integer
	float   pb.Property_FloatValue
	double  pb.Property_DoubleValue
	text    pb.Property_Text
}

func (b *batcher) slot() *recordSlot {
	n := len(b.free)
	if n == 0 {
		return new(recordSlot)
	}
	s := b.free[n-1]
	b.free = b.free[:n-1]
	return s
}
func (s *recordSlot) reset() {
	s.part = pb.Part{}
	s.node = pb.NodePart{}
	s.edge = pb.EdgePart{}
	s.nodeWrap = pb.Part_Node{}
	s.edgeWrap = pb.Part_Edge{}
	s.id = ""
	s.source = ""
	s.target = ""
	clear(s.props)
	clear(s.labels)
	clear(s.ps)
	clear(s.ls)
	s.props = s.props[:0]
	s.labels = s.labels[:0]
	s.ps = s.ps[:0]
	s.ls = s.ls[:0]
}
func (b *batcher) addSlot(s *recordSlot, size int) error {
	if e := b.add(&s.part, size); e != nil {
		return e
	}
	b.used = append(b.used, s)
	return nil
}
func (s *recordSlot) label(text string, ref uint32) *pb.Symbol {
	s.labels = append(s.labels, pb.Symbol{Text: text, Ref: ref})
	p := &s.labels[len(s.labels)-1]
	s.ls = append(s.ls, p)
	return p
}
