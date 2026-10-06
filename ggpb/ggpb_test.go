package ggpb_test

import (
	"bytes"
	"context"
	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"testing"
)

func TestParity(t *testing.T) {
	for _, name := range []string{"empty", "topology", "typed", "numeric", "partial", "presence"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			g, e := snapshot.Open(ctx, mmap.New("../fixtures/snapshot_reference/"+name+".snapshot"))
			if e != nil {
				t.Fatal(e)
			}
			defer g.Close()
			s, e := query.NewSubgraph(g)
			if e != nil {
				t.Fatal(e)
			}
			for i := uint64(0); i < g.Metadata().Nodes; i++ {
				if e = s.AddNode(graph.NodeID(i)); e != nil {
					t.Fatal(e)
				}
			}
			for i := uint64(0); i < g.Metadata().Edges; i++ {
				if e = s.AddEdge(graph.EdgeID(i)); e != nil {
					t.Fatal(e)
				}
			}
			for _, labels := range [][]string{nil, {}, {"z", "a", "z"}} {
				meta := ggpb.Query{Name: "all", Node: new(""), EdgeLabels: labels}
				var want, b, got, again bytes.Buffer
				if e = graphjson.Write(ctx, &want, g, s, graphjson.Query{Name: meta.Name, Node: meta.Node, EdgeLabels: labels}); e != nil {
					t.Fatal(e)
				}
				if e = ggpb.Write(ctx, &b, g, s, meta); e != nil {
					t.Fatal(e)
				}
				if e = ggpb.Write(ctx, &again, g, s, meta); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(b.Bytes(), again.Bytes()) {
					t.Fatal("nondeterministic")
				}
				if e = ggpb.JSON(ctx, &got, bytes.NewReader(b.Bytes())); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(want.Bytes(), got.Bytes()) {
					t.Fatalf("JSON parity\nwant %s\ngot %s", want.Bytes(), got.Bytes())
				}
			}
		})
	}
}
