package ggpb

import (
	"bytes"
	"context"
	"errors"
	"gophergraph/ggpb/pb"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestHeaderLimitBeforeEmission(t *testing.T) {
	g, s := fixture(t, "empty")
	huge := strings.Repeat("x", MaxScalar)
	calls := 0
	e := EmitEncoded(context.Background(), g, s, Query{Name: huge, Node: &huge, From: &huge, To: &huge}, Options{}, func([]byte) error { calls++; return nil })
	if !errors.Is(e, ErrLimit) || calls != 0 {
		t.Fatal(e, calls)
	}
}

func TestEncodedCallbackOwnershipAndFailure(t *testing.T) {
	g, s := fixture(t, "typed")
	var batches [][]byte
	if e := EmitEncoded(context.Background(), g, s, Query{}, Options{}, func(raw []byte) error { batches = append(batches, bytes.Clone(raw)); return nil }); e != nil {
		t.Fatal(e)
	}
	for _, raw := range batches {
		if e := proto.Unmarshal(raw, new(pb.Batch)); e != nil {
			t.Fatal("retained copy", e)
		}
	}
	if e := EmitEncoded(context.Background(), g, s, Query{}, Options{}, func([]byte) error { return outputError }); e != outputError {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	e := EmitEncoded(ctx, g, s, Query{}, Options{}, func([]byte) error { calls++; cancel(); return nil })
	if !errors.Is(e, context.Canceled) || calls != 1 {
		t.Fatal(e, calls)
	}
}

func FuzzWireScalar(f *testing.F) {
	f.Add(uint64(0x8000000000000000), int64(math.MinInt64), "")
	f.Add(uint64(0x7ff8000000000001), int64(math.MaxInt64), "é\n\"\\")
	f.Fuzz(func(t *testing.T, bits uint64, n int64, text string) {
		if len(text) > MaxScalar || !utf8.ValidString(text) {
			t.Skip()
		}
		key := &pb.Symbol{Text: text}
		ps := []*pb.Property{
			{Key: key, Kind: pb.Kind_BOOL, Value: &pb.Property_Boolean{}},
			{Key: key, Kind: pb.Kind_LONG, Value: &pb.Property_Integer{Integer: n}},
			{Key: key, Kind: pb.Kind_FLOAT, Value: &pb.Property_FloatValue{FloatValue: math.Float32frombits(uint32(bits))}},
			{Key: key, Kind: pb.Kind_DOUBLE, Value: &pb.Property_DoubleValue{DoubleValue: math.Float64frombits(bits)}},
			{Key: key, Kind: pb.Kind_STRING, Value: &pb.Property_Text{Text: text}},
		}
		b := &pb.Batch{Payload: &pb.Batch_Records{Records: &pb.Records{Parts: []*pb.Part{{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new(""), Labels: []*pb.Symbol{key}, Properties: ps, Last: true}}}}}}}
		want, e := proto.MarshalOptions{Deterministic: true}.Marshal(b)
		if e != nil {
			t.Fatal(e)
		}
		got := new(wireEncoder).batch(b)
		if !bytes.Equal(want, got) {
			t.Fatalf("wire mismatch %x %x", want, got)
		}
	})
}
