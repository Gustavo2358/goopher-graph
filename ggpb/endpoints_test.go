package ggpb

import (
	"bytes"
	"context"
	"gophergraph/ggpb/pb"
	"testing"
)

func TestLogicalEndpointReferences(t *testing.T) {
	g, s := fixture(t, "topology")
	refs := 0
	if e := Emit(context.Background(), g, s, Query{}, Options{}, func(b *pb.Batch) error {
		if r := b.GetRecords(); r != nil {
			for _, p := range r.Parts {
				if n := p.GetEdge(); n != nil && n.Id != nil {
					if n.Source != nil || n.Target != nil || n.SourceRef == 0 || n.TargetRef == 0 {
						t.Fatal("expected local references")
					}
					for _, ref := range []uint32{n.SourceRef, n.TargetRef} {
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
