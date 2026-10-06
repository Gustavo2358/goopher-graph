package ggpb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"hash/crc32"
	"io"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
)

func fixture(t testing.TB, name string) (*graph.Graph, *query.Subgraph) {
	t.Helper()
	g, e := snapshot.Open(context.Background(), mmap.New("../fixtures/snapshot_reference/"+name+".snapshot"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := g.Close(); e != nil {
			t.Error(e)
		}
	})
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
	return g, s
}
func frame(raw []byte) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(raw)))
	b = append(b, raw...)
	return binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(raw))
}
func stream(bs ...*pb.Batch) []byte {
	b := append([]byte{}, magic[:]...)
	for _, v := range bs {
		raw, _ := proto.Marshal(v)
		b = append(b, frame(raw)...)
	}
	return b
}
func read(ctx context.Context, b []byte) error {
	r := NewReader(bytes.NewReader(b))
	for {
		_, e := r.Next(ctx)
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
	}
}
func TestFramingAndValidation(t *testing.T) {
	g, s := fixture(t, "topology")
	var buf bytes.Buffer
	if e := Write(context.Background(), &buf, g, s, Query{Name: "all"}); e != nil {
		t.Fatal(e)
	}
	data := buf.Bytes()
	for n := 0; n < len(data); n++ {
		if e := read(context.Background(), data[:n]); e == nil {
			t.Fatalf("truncation accepted: %d", n)
		}
	}
	for i := range data {
		b := bytes.Clone(data)
		b[i] ^= 0x80
		if e := read(context.Background(), b); e == nil {
			t.Fatalf("corruption accepted: %d", i)
		}
	}
	if e := read(context.Background(), append(bytes.Clone(data), 0)); e == nil {
		t.Fatal("trailing bytes")
	}
	h := &pb.Batch{Payload: &pb.Batch_Header{Header: &pb.ResultHeader{Version: Version, Directed: true, Query: &pb.Query{Name: "all"}}}}
	end := &pb.Batch{Payload: &pb.Batch_End{End: &pb.ResultEnd{}}}
	rec := func(p *pb.Part) *pb.Batch {
		return &pb.Batch{Payload: &pb.Batch_Records{Records: &pb.Records{Parts: []*pb.Part{p}}}}
	}
	bad := []*pb.Batch{
		{Payload: &pb.Batch_Records{Records: &pb.Records{}}},
		rec(&pb.Part{}),
		rec(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Last: true}}}),
		rec(&pb.Part{Entity: &pb.Part_Edge{Edge: &pb.EdgePart{Id: new("e"), Last: true}}}),
		rec(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new("n"), Labels: []*pb.Symbol{{Ref: 1}}, Last: true}}}),
		rec(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new("n"), Properties: []*pb.Property{{Key: &pb.Symbol{}, Kind: pb.Kind_BYTE, Value: &pb.Property_Integer{Integer: 128}}}, Last: true}}}),
		rec(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new("n"), Properties: []*pb.Property{{Key: &pb.Symbol{}, Kind: pb.Kind_LONG, Value: &pb.Property_Text{Text: "12"}}}, Last: true}}}),
		rec(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new("n"), Last: false}}}),
		h,
	}
	for i, b := range bad {
		if e := read(context.Background(), stream(h, b, end)); e == nil {
			t.Fatal("bad logical stream", i)
		}
	}
	future := proto.Clone(h).(*pb.Batch)
	future.GetHeader().Version = 2
	if e := read(context.Background(), stream(future, end)); !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	hbad := proto.Clone(h).(*pb.Batch)
	hbad.GetHeader().Directed = false
	if e := read(context.Background(), stream(hbad, end)); e == nil {
		t.Fatal("header")
	}
	if e := read(context.Background(), append(magic[:], frame([]byte{0xff})...)); e == nil {
		t.Fatal("protobuf")
	}
	huge := append(bytes.Clone(magic[:]), binary.LittleEndian.AppendUint32(nil, MaxFrame+1)...)
	if e := read(context.Background(), huge); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	// Unknown additive fields are skipped; an unknown payload is not a v1 batch.
	raw, _ := proto.Marshal(h)
	raw = append(raw, 0x78, 0x01)
	if e := read(context.Background(), append(append(bytes.Clone(magic[:]), frame(raw)...), stream(end)[8:]...)); e != nil {
		t.Fatal(e)
	}
}

var outputError = errors.New("output failed")

type failWriter struct {
	short  bool
	cancel context.CancelFunc
	calls  int
}

func (w *failWriter) Write(b []byte) (int, error) {
	w.calls++
	if w.cancel != nil {
		w.cancel()
		return len(b), nil
	}
	if w.short {
		return len(b) / 2, nil
	}
	return 0, outputError
}
func TestErrorsAndConcurrent(t *testing.T) {
	g, s := fixture(t, "typed")
	ctx := context.Background()
	q := Query{Name: "all"}
	for _, short := range []bool{false, true} {
		w := &failWriter{short: short}
		want := outputError
		if short {
			want = io.ErrShortWrite
		}
		if e := Write(ctx, w, g, s, q); !errors.Is(e, want) {
			t.Fatal(e)
		}
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	if e := Write(c, io.Discard, g, s, q); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	c, cancel = context.WithCancel(ctx)
	w := &failWriter{cancel: cancel}
	if e := Write(c, w, g, s, q); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := NewReader(bytes.NewReader(nil)).Next(c); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := Write(ctx, io.Discard, g, nil, q); !errors.Is(e, graph.ErrGraphMismatch) {
		t.Fatal(e)
	}
	var first bytes.Buffer
	if e := Write(ctx, &first, g, s, q); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var b bytes.Buffer
			if e := Write(ctx, &b, g, s, q); e != nil || !bytes.Equal(b.Bytes(), first.Bytes()) {
				t.Error("concurrent", e)
			}
		})
	}
	wg.Wait()
	if e := g.Close(); e != nil {
		t.Fatal(e)
	}
	if e := Write(ctx, io.Discard, g, s, q); !errors.Is(e, graph.ErrClosed) {
		t.Fatal(e)
	}
}
func TestEmptyGolden(t *testing.T) {
	g, s := fixture(t, "empty")
	var b bytes.Buffer
	if e := Write(context.Background(), &b, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	// Independently specified wire fields: header(version=1, query={}, directed),
	// followed by empty end. CRC32 is part of the file contract.
	want := append(bytes.Clone(magic[:]), frame([]byte{0x0a, 0x06, 0x08, 0x01, 0x12, 0x00, 0x18, 0x01})...)
	want = append(want, frame([]byte{0x1a, 0})...)
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatal(hex.EncodeToString(b.Bytes()), hex.EncodeToString(want))
	}
}
func FuzzReader(f *testing.F) {
	g, s := fixture(f, "numeric")
	var b bytes.Buffer
	if e := Write(context.Background(), &b, g, s, Query{Name: "all"}); e != nil {
		f.Fatal(e)
	}
	f.Add(b.Bytes())
	f.Add([]byte{})
	f.Add(append(magic[:], frame([]byte{0xff})...))
	f.Fuzz(func(t *testing.T, b []byte) { _ = read(context.Background(), b) })
}

type fragmentReader struct {
	data []byte
	size int
}

func (r *fragmentReader) Read(b []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(b), r.size, len(r.data))
	copy(b, r.data[:n])
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.EOF
	}
	return n, nil
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, outputError }
func TestFragmentationAndIO(t *testing.T) {
	g, s := fixture(t, "typed")
	var buf bytes.Buffer
	if e := Write(context.Background(), &buf, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	for _, size := range []int{1, 2, 7, 4096} {
		r := NewReader(&fragmentReader{data: buf.Bytes(), size: size})
		for {
			_, e := r.Next(context.Background())
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(size, e)
			}
		}
	}
	if _, e := NewReader(brokenReader{}).Next(context.Background()); !errors.Is(e, outputError) {
		t.Fatal(e)
	}
	c, cancel := context.WithCancel(context.Background())
	calls := 0
	e := Emit(c, g, s, Query{}, Options{}, func(*pb.Batch) error { calls++; cancel(); return nil })
	if !errors.Is(e, context.Canceled) || calls != 1 {
		t.Fatal(e, calls)
	}
	for _, short := range []bool{false, true} {
		want := outputError
		if short {
			want = io.ErrShortWrite
		}
		if e := JSON(context.Background(), &failWriter{short: short}, bytes.NewReader(buf.Bytes())); !errors.Is(e, want) {
			t.Fatal(e)
		}
	}
	var wire bytes.Buffer
	if e := Write(context.Background(), &wire, g, s, Query{Name: string(make([]byte, MaxScalar+1))}); !errors.Is(e, ErrLimit) || wire.Len() != 0 {
		t.Fatal(e, wire.Len())
	}
	if e := Emit(context.Background(), g, s, Query{Name: string([]byte{0xff})}, Options{}, func(*pb.Batch) error { t.Fatal("invalid metadata emitted"); return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	// A valid scalar larger than a normal batch is emitted alone and remains bounded.
	v, _ := graph.TextValue(graph.StringKind, string(make([]byte, MaxScalar)))
	huge, sub := makeGraph(t, 1, []graph.Value{v})
	wire.Reset()
	if e := Write(context.Background(), &wire, huge, sub, Query{}); e != nil {
		t.Fatal(e)
	}
	if e := read(context.Background(), wire.Bytes()); e != nil {
		t.Fatal(e)
	}
	v, _ = graph.TextValue(graph.StringKind, string(make([]byte, MaxScalar+1)))
	huge, sub = makeGraph(t, 1, []graph.Value{v})
	if e := Write(context.Background(), io.Discard, huge, sub, Query{}); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}

func TestDictionaryPolicy(t *testing.T) {
	outputs := []*pb.Batch{}
	enc := batcher{emit: func(b *pb.Batch) error { outputs = append(outputs, proto.Clone(b).(*pb.Batch)); return nil }}
	n := &pb.NodePart{Id: new("n"), Last: true}
	for i := 0; i < 1100; i++ {
		n.Labels = append(n.Labels, &pb.Symbol{Text: fmt.Sprintf("l%04d", i)})
	}
	n.Labels = append(n.Labels, &pb.Symbol{Text: "l0000"}, &pb.Symbol{Text: strings.Repeat("x", dictionaryBytes+1)})
	if e := enc.add(&pb.Part{Entity: &pb.Part_Node{Node: n}}, 200000); e != nil {
		t.Fatal(e)
	}
	if e := enc.flush(); e != nil {
		t.Fatal(e)
	}
	r := outputs[0].GetRecords()
	if len(r.Dictionary) != maxDictionary || r.Parts[0].GetNode().Labels[1100].Ref != 1 || r.Parts[0].GetNode().Labels[1024].Ref != 0 || r.Parts[0].GetNode().Labels[1101].Ref != 0 {
		t.Fatal("dictionary limit/fallback")
	}
	if e := enc.add(&pb.Part{Entity: &pb.Part_Node{Node: &pb.NodePart{Id: new("z"), Labels: []*pb.Symbol{{Text: "l0001"}}, Last: true}}}, 32); e != nil {
		t.Fatal(e)
	}
	if e := enc.flush(); e != nil {
		t.Fatal(e)
	}
	if r = outputs[1].GetRecords(); len(r.Dictionary) != 1 || r.Dictionary[0] != "l0001" || r.Parts[0].GetNode().Labels[0].Ref != 1 {
		t.Fatal("dictionary did not reset")
	}
}
