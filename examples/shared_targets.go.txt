package sharedtargets

import (
	"context"
	"fmt"
	"gophergraph/graph"
	"gophergraph/query"
)

// Execute é uma consulta comum; não é plugin nem precisa de registry.
// O Graph pertence ao chamador e permanece aberto durante o uso do resultado.
func Execute(ctx context.Context, g *graph.Graph, a, b string) (*graph.NodeSet, error) {
	fromA, err := g.FindNode(a)
	if err != nil {
		return nil, fmt.Errorf("origem A: %w", err)
	}
	fromB, err := g.FindNode(b)
	if err != nil {
		return nil, fmt.Errorf("origem B: %w", err)
	}
	left, err := query.Reachable(ctx, g, fromA, graph.Forward, query.Options{})
	if err != nil {
		return nil, err
	}
	right, err := query.Reachable(ctx, g, fromB, graph.Forward, query.Options{})
	if err != nil {
		return nil, err
	}
	return left.Intersection(right)
}
