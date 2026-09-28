package query

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"math/rand"
	"sort"
	"testing"
)

func topology(t testing.TB, n int, edges [][3]int) *graph.Graph {
	t.Helper()
	d := graphdata.Empty()
	ss := []string{"", "L", "M"}
	for i := 0; i < n; i++ {
		ss = append(ss, fmt.Sprintf("n%08d", i))
	}
	for i := range edges {
		ss = append(ss, fmt.Sprintf("e%08d", i))
	}
	sort.Strings(ss)
	d.Strings = ss
	sid := map[string]uint32{}
	for i, s := range ss {
		sid[s] = uint32(i)
	}
	for i := 0; i < n; i++ {
		d.NodeIDs.Heap = append(d.NodeIDs.Heap, sid[fmt.Sprintf("n%08d", i)])
		d.NodeLabels.Heap = append(d.NodeLabels.Heap, sid["L"])
		d.NodeLabelOffsets.Heap = append(d.NodeLabelOffsets.Heap, uint64(i+1))
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, 0)
	}
	for i, e := range edges {
		d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, sid[fmt.Sprintf("e%08d", i)])
		d.Sources.Heap = append(d.Sources.Heap, uint32(e[0]))
		d.Targets.Heap = append(d.Targets.Heap, uint32(e[1]))
		d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, sid[[]string{"L", "M"}[e[2]]])
		d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, 0)
	}
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	g, err := graph.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestDiamondParallelCycle(t *testing.T) {
	g := topology(t, 5, [][3]int{{0, 1, 0}, {0, 2, 0}, {1, 3, 0}, {2, 3, 0}, {0, 1, 0}, {1, 0, 1}, {3, 3, 0}})
	ctx := context.Background()
	s, e := Territory(ctx, g, 0, Options{})
	if e != nil || s.NodeCount() != 4 || s.EdgeCount() != 7 {
		t.Fatal(s, e)
	}
	r, e := AntiTerritory(ctx, g, 3, Options{})
	if e != nil || r.EdgeCount() != 7 {
		t.Fatal(e)
	}
	it := r.Edges()
	if !it.Next() {
		t.Fatal("edges")
	}
	edge, _ := g.Edge(it.ID())
	if edge.Source != 0 || edge.Target != 1 {
		t.Fatal(edge)
	}
	b, e := Between(ctx, g, 0, 0, Options{})
	if e != nil || b.NodeCount() != 2 || b.EdgeCount() != 3 {
		t.Fatal(b, e)
	}
	b, e = Between(ctx, g, 0, 4, Options{})
	if e != nil || b.NodeCount() != 0 {
		t.Fatal(b, e)
	}
	for _, filter := range [][]graph.StringID{{}, {graph.InvalidStringID}} {
		r, e := Reachable(ctx, g, 0, graph.Forward, Options{filter})
		if e != nil || r.Count() != 1 {
			t.Fatal(e)
		}
	}
	nodes, _ := graph.NewNodeSet(g)
	_ = nodes.Add(0)
	sub, e := FromNodes(ctx, g, nodes, Options{})
	_ = nodes.Add(1)
	if e != nil || sub.NodeCount() != 1 {
		t.Fatal("alias", e)
	}
	other := topology(t, 5, nil)
	foreign, _ := graph.NewNodeSet(other)
	if _, e := FromNodes(ctx, g, foreign, Options{}); !errors.Is(e, graph.ErrGraphMismatch) {
		t.Fatal(e)
	}
	if e := sub.AddEdge(0); e != nil || sub.NodeCount() != 2 {
		t.Fatal("endpoints", e)
	}
}
func TestClosureOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	ctx := context.Background()
	for trial := 0; trial < 40; trial++ {
		n := 1 + rng.Intn(14)
		var edges [][3]int
		for i := 0; i < n*n; i++ {
			if rng.Intn(4) == 0 {
				edges = append(edges, [3]int{rng.Intn(n), rng.Intn(n), rng.Intn(2)})
			}
		}
		g := topology(t, n, edges)
		for _, filtered := range []bool{false, true} {
			opts := Options{}
			if filtered {
				id, _, _ := g.FindString("L")
				opts.EdgeLabels = []graph.StringID{id}
			}
			matrix := make([][]bool, n)
			for i := range matrix {
				matrix[i] = make([]bool, n)
				matrix[i][i] = true
			}
			for _, e := range edges {
				if !filtered || e[2] == 0 {
					matrix[e[0]][e[1]] = true
				}
			}
			for k := 0; k < n; k++ {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						matrix[i][j] = matrix[i][j] || matrix[i][k] && matrix[k][j]
					}
				}
			}
			for a := 0; a < n; a++ {
				for _, dir := range []graph.Direction{graph.Forward, graph.Reverse} {
					r, err := Reachable(ctx, g, graph.NodeID(a), dir, opts)
					if err != nil {
						t.Fatal(err)
					}
					for b := 0; b < n; b++ {
						want := matrix[a][b]
						if dir == graph.Reverse {
							want = matrix[b][a]
						}
						if r.Contains(graph.NodeID(b)) != want {
							t.Fatal("closure mismatch", trial, a, b, dir)
						}
					}
				}
			}
		}
	}
}

type cancelDuring struct {
	context.Context
	calls int
}

func (c *cancelDuring) Err() error {
	c.calls++
	if c.calls >= 4 {
		return context.Canceled
	}
	return nil
}
func TestCancelHub(t *testing.T) {
	edges := make([][3]int, 50000)
	for i := range edges {
		edges[i] = [3]int{0, i + 1, 0}
	}
	g := topology(t, 50001, edges)
	ctx := &cancelDuring{Context: context.Background()}
	r, e := Reachable(ctx, g, 0, graph.Forward, Options{})
	if !errors.Is(e, context.Canceled) || r != nil {
		t.Fatal("partial success", e)
	}
	ctx = &cancelDuring{Context: context.Background()}
	nodes, _ := graph.NewNodeSet(g)
	for i := 0; i < 50001; i++ {
		_ = nodes.Add(graph.NodeID(i))
	}
	s, e := FromNodes(ctx, g, nodes, Options{})
	if !errors.Is(e, context.Canceled) || s != nil {
		t.Fatal(e)
	}
}
