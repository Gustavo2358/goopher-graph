package graph

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/internal/graphdata"
	"math"
	"reflect"
	"slices"
	"testing"
)

func TestComposedSelectionAndHas(t *testing.T) {
	ctx := context.Background()
	value, _ := IntegerValue(IntKind, 1)
	for _, indexed := range []bool{false, true} {
		g := fixture(t)
		if indexed {
			g.data.IndexedKeys.Heap = []uint32{8}
			if err := rebuildSelectionIndexes(g); err != nil {
				t.Fatal(err)
			}
		}
		for _, labels := range [][]string{nil, {}, {"missing"}, {"L"}, {"M", "L", "M", "missing"}} {
			want, _ := NewNodeSet(g)
			for _, label := range labels {
				nodes, err := g.NodesWithLabel(ctx, label)
				if err != nil {
					t.Fatal(err)
				}
				global, err := g.NodesWithProperty(ctx, "p", value)
				if err != nil {
					t.Fatal(err)
				}
				selected, _ := nodes.Intersection(global)
				want, _ = want.Union(selected)
			}
			got, err := g.NodesWithAnyLabelAndProperty(ctx, labels, "p", value)
			if err != nil || !reflect.DeepEqual(got.words, want.words) {
				t.Fatalf("index=%v labels=%v got=%v want=%v err=%v", indexed, labels, got, want, err)
			}
		}
		input, _ := g.NodesWithLabel(ctx, "L")
		before := input.Clone()
		got, err := input.Has(ctx, "p", value)
		if err != nil || got.Count() != 1 || !got.Contains(0) || !reflect.DeepEqual(input.words, before.words) {
			t.Fatal(got, err, input)
		}
		otherType, _ := IntegerValue(LongKind, 1)
		for _, tc := range []struct {
			key   string
			value Value
		}{{"p", otherType}, {"missing", value}} {
			got, err := input.Has(ctx, tc.key, tc.value)
			if err != nil || got.Count() != 0 {
				t.Fatal(got, err)
			}
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if got, err := input.Has(canceled, "p", value); got != nil || !errors.Is(err, context.Canceled) {
			t.Fatal(got, err)
		}
		if got, err := g.NodesWithAnyLabelAndProperty(canceled, []string{"L"}, "p", value); got != nil || !errors.Is(err, context.Canceled) {
			t.Fatal(got, err)
		}
	}
}

// Deterministic context instrumentation makes long-loop cancellation tests
// independent of scheduling and wall-clock thresholds.
type selectionContext struct {
	context.Context
	checks, cancelAt int
	cause            error
}

func (c *selectionContext) Err() error {
	c.checks++
	if c.cancelAt > 0 && c.checks >= c.cancelAt {
		return c.cause
	}
	return nil
}

func selectionGraph(t testing.TB, n, props int, indexed bool) *Graph {
	t.Helper()
	d := graphdata.Empty()
	d.Strings = []string{"", "L", "p"}
	for i := 0; i < n; i++ {
		d.Strings = append(d.Strings, fmt.Sprintf("n%08d", i))
	}
	slices.Sort(d.Strings)
	sid := func(s string) uint32 { i, _ := slices.BinarySearch(d.Strings, s); return uint32(i) }
	for i := 0; i < n; i++ {
		d.NodeIDs.Heap = append(d.NodeIDs.Heap, sid(fmt.Sprintf("n%08d", i)))
		d.NodeLabels.Heap = append(d.NodeLabels.Heap, sid("L"))
		d.NodeLabelOffsets.Heap = append(d.NodeLabelOffsets.Heap, uint64(i+1))
		for j := 0; j < props; j++ {
			d.NodeProps.Heap = append(d.NodeProps.Heap, graphdata.Property{Key: sid("p"), Kind: 4, Payload: uint64(j)})
		}
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, uint64(len(d.NodeProps.Heap)))
	}
	if indexed {
		d.IndexedKeys.Heap = []uint32{sid("p")}
	}
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	g, err := New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestSelectionTypedMultivalues(t *testing.T) {
	values := []Value{BoolValue(true)}
	for k := ByteKind; k <= LongKind; k++ {
		v, _ := IntegerValue(k, 1)
		values = append(values, v)
	}
	for _, k := range []ValueKind{FloatKind, DoubleKind} {
		for _, f := range []float64{0, math.Copysign(0, -1), math.NaN(), math.Inf(1)} {
			v, _ := DecimalValue(k, f)
			values = append(values, v)
		}
	}
	for _, k := range []ValueKind{StringKind, DateKind, DatetimeKind} {
		v, _ := TextValue(k, "A")
		values = append(values, v)
	}
	for _, indexed := range []bool{false, true} {
		g := fixture(t)
		g.data.NodeProps.Heap = nil
		for _, v := range values {
			payload := v.payload
			if v.kind >= StringKind {
				payload = 1
			}
			g.data.NodeProps.Heap = append(g.data.NodeProps.Heap, graphdata.Property{Key: 8, Kind: uint8(v.kind), Payload: payload})
		}
		slices.SortFunc(g.data.NodeProps.Heap, func(a, b graphdata.Property) int {
			if graphdata.LessProperty(a, b) {
				return -1
			}
			if graphdata.LessProperty(b, a) {
				return 1
			}
			return 0
		})
		g.data.NodePropOffsets.Heap = []uint64{0, uint64(len(values)), uint64(len(values))}
		if indexed {
			g.data.IndexedKeys.Heap = []uint32{8}
		}
		if err := rebuildSelectionIndexes(g); err != nil {
			t.Fatal(err)
		}
		if err := graphdata.Validate(g.data); err != nil {
			t.Fatal(err)
		}
		for _, v := range values {
			got, err := g.NodesWithAnyLabelAndProperty(context.Background(), []string{"M", "L", "M"}, "p", v)
			if err != nil || got.Count() != 1 || !got.Contains(0) {
				t.Fatalf("indexed=%v value=%v got=%v err=%v", indexed, v, got, err)
			}
		}
	}
}

func TestHasWorkIsBoundedByCandidates(t *testing.T) {
	g := selectionGraph(t, 100000, 1, false)
	s, _ := NewNodeSet(g)
	_ = s.Add(0)
	value, _ := IntegerValue(IntKind, 0)
	ctx := &selectionContext{Context: context.Background()}
	got, err := s.Has(ctx, "p", value)
	// At most two checks for scanning the 1563 bitmap words, plus API, node,
	// property and completion checks. A global property scan exceeds this bound.
	if err != nil || got.Count() != 1 || ctx.checks > 8 {
		t.Fatalf("result=%v checks=%d err=%v", got, ctx.checks, err)
	}
}

func TestSelectionCancellationWithinLoops(t *testing.T) {
	value, _ := IntegerValue(IntKind, 0)
	plain := selectionGraph(t, 10000, 1, false)
	indexed := selectionGraph(t, 10000, 1, true)
	manyProps := selectionGraph(t, 1, 5000, false)
	noProps := selectionGraph(t, 10000, 0, false)
	all, _ := noProps.NodesWithLabel(context.Background(), "L")
	indexedAll, _ := indexed.NodesWithLabel(context.Background(), "L")
	one, _ := NewNodeSet(manyProps)
	_ = one.Add(0)
	sparseGraph := selectionGraph(t, 200000, 0, false)
	sparse, _ := NewNodeSet(sparseGraph)
	_ = sparse.Add(199999)
	missingValue, _ := IntegerValue(IntKind, -1)
	for _, tc := range []struct {
		name string
		at   int
		run  func(context.Context) (*NodeSet, error)
	}{
		{"label postings", 5, func(c context.Context) (*NodeSet, error) {
			return plain.NodesWithAnyLabelAndProperty(c, []string{"L"}, "missing", value)
		}},
		{"composed property postings", 15, func(c context.Context) (*NodeSet, error) {
			return indexed.NodesWithAnyLabelAndProperty(c, []string{"L"}, "p", value)
		}},
		{"composed candidate nodes", 16, func(c context.Context) (*NodeSet, error) {
			return noProps.NodesWithAnyLabelAndProperty(c, []string{"L"}, "p", value)
		}},
		{"property postings", 3, func(c context.Context) (*NodeSet, error) { return indexedAll.Has(c, "p", value) }},
		{"candidate nodes", 4, func(c context.Context) (*NodeSet, error) { return all.Has(c, "p", value) }},
		{"candidate properties", 5, func(c context.Context) (*NodeSet, error) { return one.Has(c, "p", missingValue) }},
		{"sparse bitmap", 4, func(c context.Context) (*NodeSet, error) { return sparse.Has(c, "p", value) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
				ctx := &selectionContext{Context: context.Background(), cancelAt: tc.at, cause: cause}
				got, err := tc.run(ctx)
				if got != nil || !errors.Is(err, cause) {
					t.Fatal(got, err)
				}
			}
		})
	}
}

func rebuildSelectionIndexes(g *Graph) error {
	return graphdata.BuildIndexes(context.Background(), g.data)
}

func TestSelectionEmptyAndClosedGraph(t *testing.T) {
	ctx := context.Background()
	g, err := New(graphdata.Empty(), nil)
	if err != nil {
		t.Fatal(err)
	}
	value := BoolValue(true)
	empty, err := g.NodesWithAnyLabelAndProperty(ctx, []string{"unknown"}, "flag", value)
	if err != nil || empty.Count() != 0 || !empty.BelongsTo(g) {
		t.Fatal(empty, err)
	}
	if result, err := empty.Has(ctx, "flag", value); err != nil || result.Count() != 0 {
		t.Fatal(result, err)
	}
	g.Close()
	if result, err := empty.Has(ctx, "flag", value); result != nil || !errors.Is(err, ErrClosed) {
		t.Fatal(result, err)
	}
	if result, err := g.NodesWithAnyLabelAndProperty(ctx, nil, "flag", value); result != nil || !errors.Is(err, ErrClosed) {
		t.Fatal(result, err)
	}
}
