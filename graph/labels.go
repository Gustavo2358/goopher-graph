package graph

// LabelIterator returns one node label ID at a time without a copied slice.
// The graph must remain open. It exposes no writable snapshot storage.
type LabelIterator struct {
	g        *Graph
	pos, end uint64
	id       StringID
	err      error
}

// IterateNodeLabels is the bounded-memory alternative to NodeLabels.
func (g *Graph) IterateNodeLabels(id NodeID) (LabelIterator, error) {
	if e := g.check(); e != nil {
		return LabelIterator{}, e
	}
	if uint64(id) >= g.metadata.Nodes {
		return LabelIterator{}, ErrOutOfRange
	}
	return LabelIterator{g: g, pos: g.data.NodeLabelOffsets.At(uint64(id)), end: g.data.NodeLabelOffsets.At(uint64(id) + 1)}, nil
}
func (it *LabelIterator) Next() bool {
	if it.err != nil {
		return false
	}
	if e := it.g.check(); e != nil {
		it.err = e
		return false
	}
	if it.pos >= it.end {
		return false
	}
	it.id = StringID(it.g.data.NodeLabels.At(it.pos))
	it.pos++
	return true
}
func (it *LabelIterator) ID() StringID { return it.id }
func (it *LabelIterator) Err() error   { return it.err }
