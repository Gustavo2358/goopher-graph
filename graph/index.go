package graph

import (
	"context"
	"gophergraph/internal/graphdata"
	"sort"
)

func (g *Graph) PropertyIndexed(key string) (bool, error) {
	id, ok, e := g.FindString(key)
	if e != nil || !ok {
		return false, e
	}
	return graphdata.HasU32(g.data.IndexedKeys, 0, g.data.IndexedKeys.Len(), uint32(id)), nil
}
func (g *Graph) labelPosting(owner uint32, label string) (uint64, uint64, error) {
	id, ok, e := g.FindString(label)
	if e != nil || !ok {
		return 0, 0, e
	}
	c := g.data.LabelIndex
	i := sort.Search(int(c.Len()), func(i int) bool {
		v := c.At(uint64(i))
		return v.Owner > owner || (v.Owner == owner && v.Label >= uint32(id))
	})
	if uint64(i) < c.Len() {
		v := c.At(uint64(i))
		if v.Owner == owner && v.Label == uint32(id) {
			return v.Start, v.Count, nil
		}
	}
	return 0, 0, nil
}
func (g *Graph) propertyPosting(owner uint8, key string, value Value) (uint64, uint64, bool, error) {
	id, ok, e := g.FindString(key)
	if e != nil || !ok {
		return 0, 0, true, e
	}
	p := graphdata.Property{Key: uint32(id), Kind: uint8(value.kind), Payload: value.payload}
	if value.kind >= StringKind {
		sid, ok, e := g.FindString(value.text)
		if e != nil || !ok {
			return 0, 0, true, e
		}
		p.Payload = uint64(sid)
	}
	if !graphdata.HasU32(g.data.IndexedKeys, 0, g.data.IndexedKeys.Len(), uint32(id)) {
		return 0, 0, false, nil
	}
	c := g.data.PropertyIndex
	i := sort.Search(int(c.Len()), func(i int) bool {
		v := c.At(uint64(i))
		return v.Owner > owner || (v.Owner == owner && !graphdata.LessProperty(v.Property, p))
	})
	if uint64(i) < c.Len() {
		v := c.At(uint64(i))
		if v.Owner == owner && v.Property == p {
			return v.Start, v.Count, true, nil
		}
	}
	return 0, 0, true, nil
}

func (g *Graph) NodesWithLabel(ctx context.Context, label string) (*NodeSet, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	set, e := NewNodeSet(g)
	if e != nil {
		return nil, e
	}
	start, count, e := g.labelPosting(0, label)
	if e != nil {
		return nil, e
	}
	for i := uint64(0); i < count; i++ {
		if i%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		_ = set.Add(NodeID(g.data.LabelPostings.At(start + i)))
	}
	return set, nil
}
func (g *Graph) NodesWithProperty(ctx context.Context, key string, value Value) (*NodeSet, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	set, e := NewNodeSet(g)
	if e != nil {
		return nil, e
	}
	start, count, indexed, e := g.propertyPosting(0, key, value)
	if e != nil {
		return nil, e
	}
	if indexed {
		for i := uint64(0); i < count; i++ {
			if i%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			_ = set.Add(NodeID(g.data.PropertyPostings.At(start + i)))
		}
		return set, nil
	}
	keyID, ok, e := g.FindString(key)
	if e != nil {
		return nil, e
	}
	if !ok {
		return set, nil
	}
	var steps uint64
	for i := uint64(0); i < g.metadata.Nodes; i++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		it, e := g.NodeProperties(NodeID(i))
		if e != nil {
			return nil, e
		}
		for it.Next() {
			steps++
			if steps%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			p := it.Property()
			if p.Key == keyID && p.Value.Equal(value) {
				_ = set.Add(NodeID(i))
				break
			}
		}
		if e := it.Err(); e != nil {
			return nil, e
		}
	}
	return set, nil
}

func (g *Graph) EdgesWithLabel(ctx context.Context, label string) (*EdgeSet, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	set, e := NewEdgeSet(g)
	if e != nil {
		return nil, e
	}
	start, count, e := g.labelPosting(1, label)
	if e != nil {
		return nil, e
	}
	for i := uint64(0); i < count; i++ {
		if i%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		_ = set.Add(EdgeID(g.data.LabelPostings.At(start + i)))
	}
	return set, nil
}
func (g *Graph) EdgesWithProperty(ctx context.Context, key string, value Value) (*EdgeSet, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	set, e := NewEdgeSet(g)
	if e != nil {
		return nil, e
	}
	start, count, indexed, e := g.propertyPosting(1, key, value)
	if e != nil {
		return nil, e
	}
	if indexed {
		for i := uint64(0); i < count; i++ {
			if i%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			_ = set.Add(EdgeID(g.data.PropertyPostings.At(start + i)))
		}
		return set, nil
	}
	keyID, ok, e := g.FindString(key)
	if e != nil {
		return nil, e
	}
	if !ok {
		return set, nil
	}
	var steps uint64
	for i := uint64(0); i < g.metadata.Edges; i++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		it, e := g.EdgeProperties(EdgeID(i))
		if e != nil {
			return nil, e
		}
		for it.Next() {
			steps++
			if steps%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			p := it.Property()
			if p.Key == keyID && p.Value.Equal(value) {
				_ = set.Add(EdgeID(i))
				break
			}
		}
		if e := it.Err(); e != nil {
			return nil, e
		}
	}
	return set, nil
}
