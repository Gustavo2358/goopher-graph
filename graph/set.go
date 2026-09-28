package graph

import "math/bits"

type NodeSet struct {
	g     *Graph
	words []uint64
	size  uint64
}

func NewNodeSet(g *Graph) (*NodeSet, error) {
	if e := g.check(); e != nil {
		return nil, e
	}
	n := g.metadata.Nodes
	return &NodeSet{g, make([]uint64, (n+63)/64), n}, nil
}
func (s *NodeSet) Add(id NodeID) error {
	if uint64(id) >= s.size {
		return ErrOutOfRange
	}
	s.words[id/64] |= uint64(1) << (id % 64)
	return nil
}
func (s *NodeSet) Contains(id NodeID) bool {
	return uint64(id) < s.size && s.words[id/64]&(uint64(1)<<(id%64)) != 0
}
func (s *NodeSet) Count() uint64 {
	var n uint64
	for _, v := range s.words {
		n += uint64(bits.OnesCount64(v))
	}
	return n
}
func (s *NodeSet) Clone() *NodeSet         { return &NodeSet{s.g, append([]uint64(nil), s.words...), s.size} }
func (s *NodeSet) BelongsTo(g *Graph) bool { return s != nil && s.g == g }
func (s *NodeSet) Intersection(other *NodeSet) (*NodeSet, error) {
	if other == nil || s.g != other.g {
		return nil, ErrGraphMismatch
	}
	out := s.Clone()
	for i := range out.words {
		out.words[i] &= other.words[i]
	}
	return out, nil
}
func (s *NodeSet) Union(other *NodeSet) (*NodeSet, error) {
	if other == nil || s.g != other.g {
		return nil, ErrGraphMismatch
	}
	out := s.Clone()
	for i := range out.words {
		out.words[i] |= other.words[i]
	}
	return out, nil
}

type NodeIterator struct {
	s         *NodeSet
	word      int
	remaining uint64
	id        NodeID
}

func (s *NodeSet) Iterator() NodeIterator { return NodeIterator{s: s} }
func (it *NodeIterator) Next() bool {
	if it.s == nil {
		return false
	}
	for it.remaining == 0 {
		if it.word >= len(it.s.words) {
			return false
		}
		it.remaining = it.s.words[it.word]
		it.word++
	}
	b := bits.TrailingZeros64(it.remaining)
	it.id = NodeID((it.word-1)*64 + b)
	it.remaining &= it.remaining - 1
	return true
}
func (it *NodeIterator) ID() NodeID { return it.id }

type EdgeSet struct {
	g     *Graph
	words []uint64
	size  uint64
}

func NewEdgeSet(g *Graph) (*EdgeSet, error) {
	if e := g.check(); e != nil {
		return nil, e
	}
	n := g.metadata.Edges
	return &EdgeSet{g, make([]uint64, (n+63)/64), n}, nil
}
func (s *EdgeSet) Add(id EdgeID) error {
	if uint64(id) >= s.size {
		return ErrOutOfRange
	}
	s.words[id/64] |= uint64(1) << (id % 64)
	return nil
}
func (s *EdgeSet) Contains(id EdgeID) bool {
	return uint64(id) < s.size && s.words[id/64]&(uint64(1)<<(id%64)) != 0
}
func (s *EdgeSet) Count() uint64 {
	var n uint64
	for _, v := range s.words {
		n += uint64(bits.OnesCount64(v))
	}
	return n
}
func (s *EdgeSet) Clone() *EdgeSet         { return &EdgeSet{s.g, append([]uint64(nil), s.words...), s.size} }
func (s *EdgeSet) BelongsTo(g *Graph) bool { return s != nil && s.g == g }
func (s *EdgeSet) Intersection(other *EdgeSet) (*EdgeSet, error) {
	if other == nil || s.g != other.g {
		return nil, ErrGraphMismatch
	}
	out := s.Clone()
	for i := range out.words {
		out.words[i] &= other.words[i]
	}
	return out, nil
}
func (s *EdgeSet) Union(other *EdgeSet) (*EdgeSet, error) {
	if other == nil || s.g != other.g {
		return nil, ErrGraphMismatch
	}
	out := s.Clone()
	for i := range out.words {
		out.words[i] |= other.words[i]
	}
	return out, nil
}

type EdgeIDIterator struct {
	s         *EdgeSet
	word      int
	remaining uint64
	id        EdgeID
}

func (s *EdgeSet) Iterator() EdgeIDIterator { return EdgeIDIterator{s: s} }
func (it *EdgeIDIterator) Next() bool {
	if it.s == nil {
		return false
	}
	for it.remaining == 0 {
		if it.word >= len(it.s.words) {
			return false
		}
		it.remaining = it.s.words[it.word]
		it.word++
	}
	b := bits.TrailingZeros64(it.remaining)
	it.id = EdgeID((it.word-1)*64 + b)
	it.remaining &= it.remaining - 1
	return true
}
func (it *EdgeIDIterator) ID() EdgeID { return it.id }
