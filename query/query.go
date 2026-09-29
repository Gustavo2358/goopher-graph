// Package query composes adjacency traversal and graph-bound result sets.
package query

import (
	"context"
	"gophergraph/graph"
)

type Options struct{ EdgeLabels []graph.StringID }

func (o Options) allows(label graph.StringID) bool {
	if o.EdgeLabels == nil {
		return true
	}
	for _, v := range o.EdgeLabels {
		if v == label {
			return true
		}
	}
	return false
}

type traversalStats struct {
	expansions  []uint32
	adjacencies uint64
}

func Reachable(ctx context.Context, g *graph.Graph, start graph.NodeID, direction graph.Direction, options Options) (*graph.NodeSet, error) {
	return reachable(ctx, g, start, direction, options, nil)
}
func reachable(ctx context.Context, g *graph.Graph, start graph.NodeID, direction graph.Direction, options Options, stats *traversalStats) (*graph.NodeSet, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	visited, e := graph.NewNodeSet(g)
	if e != nil {
		return nil, e
	}
	if e = visited.Add(start); e != nil {
		return nil, e
	}
	if direction > graph.Reverse {
		return nil, graph.ErrOutOfRange
	}
	queue := []graph.NodeID{start}
	var steps uint64
	for head := 0; head < len(queue); head++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if stats != nil {
			stats.expansions[queue[head]]++
		}
		it, e := g.Adjacent(queue[head], direction)
		if e != nil {
			return nil, e
		}
		for it.Next() {
			steps++
			if stats != nil {
				stats.adjacencies++
			}
			if steps%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			edge := it.Edge()
			if !options.allows(edge.Label) || visited.Contains(edge.Neighbor) {
				continue
			}
			_ = visited.Add(edge.Neighbor)
			queue = append(queue, edge.Neighbor)
		}
		if e := it.Err(); e != nil {
			return nil, e
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return visited, nil
}

type Subgraph struct {
	g     *graph.Graph
	nodes *graph.NodeSet
	edges *graph.EdgeSet
}

func NewSubgraph(g *graph.Graph) (*Subgraph, error) {
	nodes, e := graph.NewNodeSet(g)
	if e != nil {
		return nil, e
	}
	edges, e := graph.NewEdgeSet(g)
	if e != nil {
		return nil, e
	}
	return &Subgraph{g, nodes, edges}, nil
}
func (s *Subgraph) AddNode(id graph.NodeID) error { return s.nodes.Add(id) }
func (s *Subgraph) AddEdge(id graph.EdgeID) error {
	edge, e := s.g.Edge(id)
	if e != nil {
		return e
	}
	_ = s.nodes.Add(edge.Source)
	_ = s.nodes.Add(edge.Target)
	return s.edges.Add(id)
}
func (s *Subgraph) Nodes() graph.NodeIterator     { return s.nodes.Iterator() }
func (s *Subgraph) Edges() graph.EdgeIDIterator   { return s.edges.Iterator() }
func (s *Subgraph) NodeCount() uint64             { return s.nodes.Count() }
func (s *Subgraph) EdgeCount() uint64             { return s.edges.Count() }
func (s *Subgraph) BelongsTo(g *graph.Graph) bool { return s != nil && s.g == g }
func FromNodes(ctx context.Context, g *graph.Graph, nodes *graph.NodeSet, options Options) (*Subgraph, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !nodes.BelongsTo(g) {
		return nil, graph.ErrGraphMismatch
	}
	s, e := NewSubgraph(g)
	if e != nil {
		return nil, e
	}
	s.nodes = nodes.Clone()
	it := s.nodes.Iterator()
	var steps uint64
	for it.Next() {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		adj, e := g.Adjacent(it.ID(), graph.Forward)
		if e != nil {
			return nil, e
		}
		for adj.Next() {
			steps++
			if steps%1024 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			edge := adj.Edge()
			if s.nodes.Contains(edge.Target) && options.allows(edge.Label) {
				_ = s.edges.Add(edge.ID)
			}
		}
		if e := adj.Err(); e != nil {
			return nil, e
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return s, nil
}
func Territory(ctx context.Context, g *graph.Graph, start graph.NodeID, o Options) (*Subgraph, error) {
	n, e := Reachable(ctx, g, start, graph.Forward, o)
	if e != nil {
		return nil, e
	}
	return FromNodes(ctx, g, n, o)
}
func AntiTerritory(ctx context.Context, g *graph.Graph, start graph.NodeID, o Options) (*Subgraph, error) {
	n, e := Reachable(ctx, g, start, graph.Reverse, o)
	if e != nil {
		return nil, e
	}
	return FromNodes(ctx, g, n, o)
}
func Between(ctx context.Context, g *graph.Graph, from, to graph.NodeID, o Options) (*Subgraph, error) {
	a, e := Reachable(ctx, g, from, graph.Forward, o)
	if e != nil {
		return nil, e
	}
	b, e := Reachable(ctx, g, to, graph.Reverse, o)
	if e != nil {
		return nil, e
	}
	n, e := a.Intersection(b)
	if e != nil {
		return nil, e
	}
	return FromNodes(ctx, g, n, o)
}
