package graph

import (
	"context"
	"math/bits"
)

// NodesWithAnyLabelAndProperty selects the union of labels AND a typed property
// match. Nil/empty labels select no nodes. Unknown/repeated labels are harmless.
// It allocates two node bitmaps: candidates and the independently owned result.
func (g *Graph) NodesWithAnyLabelAndProperty(ctx context.Context, labels []string, key string, value Value) (*NodeSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates, err := NewNodeSet(g)
	if err != nil {
		return nil, err
	}
	for _, label := range labels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start, count, err := g.labelPosting(0, label)
		if err != nil {
			return nil, err
		}
		for i := uint64(0); i < count; i++ {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			_ = candidates.Add(NodeID(g.data.LabelPostings.At(start + i)))
		}
	}
	return candidates.Has(ctx, key, value)
}

// Has returns a new set, preserving the input. Indexed properties intersect
// postings directly; otherwise only selected nodes' properties are examined.
// Multivalued properties match when any value has the existing typed equality.
// Only the result bitmap is allocated; no global property bitmap is created.
func (s *NodeSet) Has(ctx context.Context, key string, value Value) (*NodeSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := NewNodeSet(s.g)
	if err != nil {
		return nil, err
	}
	// Find the first candidate before looking up a property. Empty sets do no
	// property work, and sparse scans can start at this first nonempty word.
	first := 0
	for first < len(s.words) && s.words[first] == 0 {
		if first%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		first++
	}
	if first == len(s.words) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	start, count, indexed, err := s.g.propertyPosting(0, key, value)
	if err != nil {
		return nil, err
	}
	if indexed {
		for i := uint64(0); i < count; i++ {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			id := NodeID(s.g.data.PropertyPostings.At(start + i))
			if s.Contains(id) {
				_ = out.Add(id)
			}
		}
	} else {
		keyID, found, err := s.g.FindString(key)
		if err != nil {
			return nil, err
		}
		if found {
			var nodes, properties uint64
			// Scan bitmap words explicitly: Iterator.Next can cross a large empty span
			// without returning control to the caller to check cancellation.
			for offset, members := range s.words[first:] {
				word := first + offset
				if word%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				for members != 0 {
					if nodes%1024 == 0 {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
					}
					nodes++
					id := NodeID(uint64(word)*64 + uint64(bits.TrailingZeros64(members)))
					members &= members - 1
					it, err := s.g.NodeProperties(id)
					if err != nil {
						return nil, err
					}
					for it.Next() {
						if properties%1024 == 0 {
							if err := ctx.Err(); err != nil {
								return nil, err
							}
						}
						properties++
						p := it.Property()
						if p.Key == keyID && p.Value.Equal(value) {
							_ = out.Add(id)
							break
						}
					}
					if err := it.Err(); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
