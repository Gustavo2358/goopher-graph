package ggpb

import (
	"bytes"
	"context"
	"gophergraph/ggpb/pb"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestWireMatchesOfficialProtobuf(t *testing.T) {
	for _, name := range []string{"empty", "topology", "typed", "numeric", "partial", "presence"} {
		t.Run(name, func(t *testing.T) {
			g, s := fixture(t, name)
			for _, inline := range []bool{false, true} {
				var w wireEncoder
				if e := Emit(context.Background(), g, s, Query{Name: name, EdgeLabels: []string{}}, Options{InlineSymbols: inline}, func(b *pb.Batch) error {
					want, e := proto.MarshalOptions{Deterministic: true}.Marshal(b)
					if e != nil {
						return e
					}
					got := w.batch(b)
					if !bytes.Equal(want, got) {
						t.Fatalf("wire differs: %x / %x", want, got)
					}
					decoded := new(pb.Batch)
					if e = proto.Unmarshal(got, decoded); e != nil {
						return e
					}
					if !proto.Equal(b, decoded) {
						t.Fatal("semantic wire mismatch")
					}
					return nil
				}); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
