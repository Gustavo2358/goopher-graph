package ggpb

import (
	"bytes"
	"context"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"gophergraph/query"
	"testing"
)

func TestLogicalEndpointReferences(t *testing.T) {
	g, s := fixture(t, "topology")
	refs := 0
	if e := Emit(context.Background(), g, s, Query{}, Options{}, func(b *pb.Batch) error {
		if r := b.GetRecords(); r != nil {
			for _, p := range r.Parts {
				if n := p.GetEdge(); n != nil && n.Id != nil {
					id, e := g.FindEdge(*n.Id)
					if e != nil {
						return e
					}
					edge, e := g.Edge(id)
					if e != nil {
						return e
					}
					source, e := endpoint(n.Source, n.SourceRef, r.EndpointIds)
					if e != nil {
						return e
					}
					target, e := endpoint(n.Target, n.TargetRef, r.EndpointIds)
					if e != nil {
						return e
					}
					expectedSource, e := g.NodeExternalID(edge.Source)
					if e != nil {
						return e
					}
					expectedTarget, e := g.NodeExternalID(edge.Target)
					if e != nil {
						return e
					}
					if source != expectedSource || target != expectedTarget {
						t.Fatal("logical ref leaks/wrong endpoint")
					}

					for _, ref := range []uint32{n.SourceRef, n.TargetRef} {
						if ref == 0 {
							continue
						}
						if int(ref) > len(r.EndpointIds) {
							t.Fatal("unresolved ref")
						}
						refs++
					}
				}
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if refs == 0 {
		t.Fatal("no refs")
	}
	// A reference cannot name a prior batch or coexist with an inline endpoint.
	for _, edge := range []*pb.EdgePart{
		{Id: new("e"), SourceRef: 2, TargetRef: 1, Label: &pb.Symbol{}, Last: true},
		{Id: new("e"), Source: new("n"), SourceRef: 1, TargetRef: 1, Label: &pb.Symbol{}, Last: true},
	} {
		r := &pb.Records{EndpointIds: []string{"n"}, Parts: []*pb.Part{{Entity: &pb.Part_Edge{Edge: edge}}}}
		if e := new(validator).records(context.Background(), r); e == nil {
			t.Fatal("bad endpoint accepted")
		}
	}
	var a, b bytes.Buffer
	if e := Write(context.Background(), &a, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	if e := Write(context.Background(), &b, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("determinism")
	}
}

// Policy is legitimate protocol fallback, not a relaxed semantic check: one use
// must stay inline, repeated uses must save bytes and use local logical refs.
func TestEndpointPolicy(t *testing.T) {
	for _, repeat := range []int{1, 2} {
		d := graphdata.Empty()
		d.Strings = []string{"", "L", "e0", "e1", "nsource", "ntarget"}
		d.NodeIDs.Heap = []uint32{4, 5}
		d.NodeLabels.Heap = []uint32{1, 1}
		d.NodeLabelOffsets.Heap = []uint64{0, 1, 2}
		d.NodePropOffsets.Heap = []uint64{0, 0, 0}
		for i := 0; i < repeat; i++ {
			d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, uint32(i+2))
			d.Sources.Heap = append(d.Sources.Heap, 0)
			d.Targets.Heap = append(d.Targets.Heap, 1)
			d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, 1)
			d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, 0)
		}
		graphdata.BuildCSR(d)
		if e := graphdata.BuildIndexes(context.Background(), d); e != nil {
			t.Fatal(e)
		}
		g, e := graph.New(d, nil)
		if e != nil {
			t.Fatal(e)
		}
		s, e := query.NewSubgraph(g)
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 2; i++ {
			if e := s.AddNode(graph.NodeID(i)); e != nil {
				t.Fatal(e)
			}
		}
		for i := 0; i < repeat; i++ {
			if e := s.AddEdge(graph.EdgeID(i)); e != nil {
				t.Fatal(e)
			}
		}
		if e := Emit(context.Background(), g, s, Query{}, Options{}, func(b *pb.Batch) error {
			if r := b.GetRecords(); r != nil {
				for _, p := range r.Parts {
					if n := p.GetEdge(); n != nil {
						if repeat == 1 {
							if n.SourceRef != 0 || n.TargetRef != 0 || n.Source == nil || n.Target == nil || len(r.EndpointIds) != 0 {
								t.Fatal("singletons expanded")
							}
						}
						if repeat == 2 {
							if n.SourceRef == 0 || n.TargetRef == 0 || n.Source != nil || n.Target != nil || len(r.EndpointIds) != 2 {
								t.Fatal("repeated endpoints not interned")
							}
						}
					}
				}
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		if e := g.Close(); e != nil {
			t.Fatal(e)
		}
	}
}
