package graph

import (
	"context"
	"errors"
	"gophergraph/internal/graphdata"
	"math"
	"testing"
)

func fixture(t *testing.T) *Graph {
	t.Helper()
	d := graphdata.Empty()
	d.Strings = []string{"", "A", "B", "L", "M", "e0", "e1", "loop", "p"}
	d.NodeIDs = graphdata.U32{Heap: []uint32{1, 2}}
	d.NodeLabelOffsets = graphdata.U64{Heap: []uint64{0, 2, 3}}
	d.NodeLabels = graphdata.U32{Heap: []uint32{3, 4, 3}}
	d.NodePropOffsets = graphdata.U64{Heap: []uint64{0, 1, 1}}
	d.NodeProps = graphdata.Properties{Heap: []graphdata.Property{{Key: 8, Kind: 4, Payload: 1}}}
	d.EdgeIDs = graphdata.U32{Heap: []uint32{5, 6, 7}}
	d.Sources = graphdata.U32{Heap: []uint32{0, 0, 1}}
	d.Targets = graphdata.U32{Heap: []uint32{1, 1, 1}}
	d.EdgeLabels = graphdata.U32{Heap: []uint32{3, 3, 4}}
	d.EdgePropOffsets = graphdata.U64{Heap: []uint64{0, 0, 0, 0}}
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	g, err := New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestGraph(t *testing.T) {
	g := fixture(t)
	if n, e := g.FindNode("A"); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if _, e := g.FindNode(""); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	labels, _ := g.NodeLabels(0)
	labels[0] = 99
	labels, _ = g.NodeLabels(0)
	if labels[0] != 3 {
		t.Fatal("mutable alias")
	}
	it, e := g.Adjacent(1, Reverse)
	if e != nil {
		t.Fatal(e)
	}
	var ids []EdgeID
	for it.Next() {
		v := it.Edge()
		ids = append(ids, v.ID)
		if v.Target != 1 || v.Neighbor != v.Source {
			t.Fatal(v)
		}
	}
	if len(ids) != 3 || it.Err() != nil {
		t.Fatal(ids, it.Err())
	}
	if _, e := g.Edge(InvalidEdgeID); !errors.Is(e, ErrOutOfRange) {
		t.Fatal(e)
	}
	if _, e := g.Adjacent(0, Direction(3)); !errors.Is(e, ErrOutOfRange) {
		t.Fatal(e)
	}
	p, _ := g.NodeProperties(0)
	if !p.Next() {
		t.Fatal("missing property")
	}
	v, _ := IntegerValue(IntKind, 1)
	if !p.Property().Value.Equal(v) {
		t.Fatal(p.Property())
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if it.Next() || !errors.Is(it.Err(), ErrClosed) {
		t.Fatal("iterator after close")
	}
	if _, e := g.FindNode("A"); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if g.Metadata().Nodes != 2 {
		t.Fatal("metadata lost")
	}
}
func TestSetsAndEmpty(t *testing.T) {
	g := fixture(t)
	s, _ := NewNodeSet(g)
	_ = s.Add(1)
	_ = s.Add(1)
	c := s.Clone()
	_ = c.Add(0)
	if s.Count() != 1 || c.Count() != 2 || s.Contains(InvalidNodeID) {
		t.Fatal("set membership")
	}
	other, _ := NewNodeSet(fixture(t))
	if _, e := s.Union(other); !errors.Is(e, ErrGraphMismatch) {
		t.Fatal(e)
	}
	i := c.Iterator()
	for n := NodeID(0); n < 2; n++ {
		if !i.Next() || i.ID() != n {
			t.Fatal("iteration")
		}
	}
	if i.Next() {
		t.Fatal("extra")
	}
	e, _ := NewEdgeSet(g)
	_ = e.Add(2)
	if e.Count() != 1 || e.Contains(0) {
		t.Fatal("edge set")
	}
	empty, err := New(graphdata.Empty(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ns, _ := NewNodeSet(empty)
	if ns.Count() != 0 || ns.Add(0) == nil {
		t.Fatal("empty")
	}
	var zero Graph
	if _, err := NewNodeSet(&zero); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	releases := 0
	owned, err := New(graphdata.Empty(), func() error { releases++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	_ = owned.Close()
	_ = owned.Close()
	if releases != 1 {
		t.Fatal(releases)
	}
}
func TestValues(t *testing.T) {
	for k := ByteKind; k <= LongKind; k++ {
		v, e := IntegerValue(k, -1)
		n, ok := v.Int64()
		if e != nil || !ok || n != -1 {
			t.Fatal(k, v, e)
		}
	}
	if _, e := IntegerValue(ByteKind, 128); e == nil {
		t.Fatal("overflow")
	}
	if _, e := TextValue(StringKind, string([]byte{255})); e == nil {
		t.Fatal("utf8")
	}
	for _, k := range []ValueKind{FloatKind, DoubleKind} {
		a, _ := DecimalValue(k, math.NaN())
		b, _ := DecimalValue(k, math.Float64frombits(0xfff8000000000001))
		if !a.Equal(b) {
			t.Fatal("NaN")
		}
		a, _ = DecimalValue(k, 0)
		b, _ = DecimalValue(k, math.Copysign(0, -1))
		if a.Equal(b) {
			t.Fatal("signed zero")
		}
	}
	a, _ := IntegerValue(IntKind, 1)
	b, _ := IntegerValue(LongKind, 1)
	if a.Equal(b) {
		t.Fatal("tags")
	}
	for _, k := range []ValueKind{StringKind, DateKind, DatetimeKind} {
		v, e := TextValue(k, "")
		s, ok := v.Text()
		if e != nil || !ok || s != "" {
			t.Fatal(k)
		}
	}
}
func TestAdjacencyNoPerEdgeAllocation(t *testing.T) {
	g := fixture(t)
	n := testing.AllocsPerRun(100, func() {
		it, err := g.Adjacent(0, Forward)
		if err != nil {
			panic(err)
		}
		for it.Next() {
			_ = it.Edge()
		}
	})
	if n != 0 {
		t.Fatalf("allocations=%v", n)
	}
}
func TestPostingMembershipValidation(t *testing.T) {
	for _, mutate := range []func(*graphdata.Data){func(d *graphdata.Data) { d.LabelPostings.Heap[0] = 1; d.LabelPostings.Heap[1] = 1 }, func(d *graphdata.Data) { d.LabelIndex.Heap[0].Count-- }, func(d *graphdata.Data) {
		d.IndexedKeys.Heap = []uint32{8}
		d.PropertyIndex = graphdata.PropertyEntries{}
	}} {
		g := fixture(t)
		mutate(g.data)
		if err := graphdata.Validate(g.data); err == nil {
			t.Fatal("invalid postings accepted")
		}
	}
}
