package ingest_test

import (
	"context"
	"gophergraph/graph"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/internal/testutil"
	"path/filepath"
	"testing"
)

func TestIndexMatchesScan(t *testing.T) {
	paths, _ := filepath.Glob("../fixtures/*/expected.json")
	ctx := context.Background()
	for _, p := range paths {
		dir := filepath.Dir(p)
		w := testutil.Read(t, dir)
		var keys []string
		for _, n := range w.Graph.Nodes {
			for _, p := range n.Properties {
				keys = append(keys, p.Key)
			}
		}
		for _, e := range w.Graph.Edges {
			for _, p := range e.Properties {
				keys = append(keys, p.Key)
			}
		}
		keys = append(keys, "unused")
		n, e := catalog(t, filepath.Join(dir, "nodes")), catalog(t, filepath.Join(dir, "edges"))
		indexed, _, err := ingest.Build(ctx, n, e, neptune.Decoder{}, &diagnostics{}, ingest.Options{IndexProperties: keys})
		if err != nil {
			t.Fatal(err)
		}
		scan, _, err := ingest.Build(ctx, n, e, neptune.Decoder{}, &diagnostics{}, ingest.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if ok, e := indexed.PropertyIndexed("unused"); e != nil || !ok {
			t.Fatal("empty index", e)
		}
		check := func(p testutil.Property) {
			v := testutil.Value(t, p)
			a, err := indexed.NodesWithProperty(ctx, p.Key, v)
			if err != nil {
				t.Fatal(err)
			}
			b, err := scan.NodesWithProperty(ctx, p.Key, v)
			if err != nil {
				t.Fatal(err)
			}
			if a.Count() != b.Count() {
				t.Fatal(p)
			}
			for i := uint64(0); i < indexed.Metadata().Nodes; i++ {
				if a.Contains(graph.NodeID(i)) != b.Contains(graph.NodeID(i)) {
					t.Fatal("node index", p)
				}
			}
			x, err := indexed.EdgesWithProperty(ctx, p.Key, v)
			if err != nil {
				t.Fatal(err)
			}
			y, err := scan.EdgesWithProperty(ctx, p.Key, v)
			if err != nil {
				t.Fatal(err)
			}
			for i := uint64(0); i < indexed.Metadata().Edges; i++ {
				if x.Contains(graph.EdgeID(i)) != y.Contains(graph.EdgeID(i)) {
					t.Fatal("edge index", p)
				}
			}
		}
		for _, n := range w.Graph.Nodes {
			for _, p := range n.Properties {
				check(p)
			}
			for _, l := range n.Labels {
				s, e := indexed.NodesWithLabel(ctx, l)
				if e != nil {
					t.Fatal(e)
				}
				id, _ := indexed.FindNode(n.ID)
				if !s.Contains(id) {
					t.Fatal("label index")
				}
			}
		}
		for _, e := range w.Graph.Edges {
			for _, p := range e.Properties {
				check(p)
			}
			s, err := indexed.EdgesWithLabel(ctx, e.Label)
			id, _ := indexed.FindEdge(e.ID)
			if err != nil || !s.Contains(id) {
				t.Fatal(err)
			}
		}
		none, err := indexed.NodesWithLabel(ctx, "not-present")
		if err != nil || none.Count() != 0 {
			t.Fatal(err)
		}
		_ = indexed.Close()
		if _, e := indexed.NodesWithProperty(ctx, "missing", graph.BoolValue(true)); e != graph.ErrClosed {
			t.Fatal(e)
		}
	}
}
