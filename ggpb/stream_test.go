package ggpb

import (
	"bytes"
	"context"
	"fmt"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/internal/graphdata"
	"gophergraph/internal/testutil"
	"gophergraph/query"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func makeGraph(t testing.TB, n int, values []graph.Value) (*graph.Graph, *query.Subgraph) {
	t.Helper()
	d := graphdata.Empty()
	d.Strings = []string{"", "L", "payload"}
	for i := range values {
		d.Strings = append(d.Strings, fmt.Sprintf("p%09d", i))
	}
	for _, v := range values {
		if s, ok := v.Text(); ok {
			d.Strings = append(d.Strings, s)
		}
	}
	for i := 0; i < n; i++ {
		d.Strings = append(d.Strings, fmt.Sprintf("n%09d", i), fmt.Sprintf("e%09d", i))
	}
	slices.Sort(d.Strings)
	d.Strings = slices.Compact(d.Strings)
	sid := func(s string) uint32 {
		i, ok := slices.BinarySearch(d.Strings, s)
		if !ok {
			t.Fatal(s)
		}
		return uint32(i)
	}
	for i := 0; i < n; i++ {
		d.NodeIDs.Heap = append(d.NodeIDs.Heap, sid(fmt.Sprintf("n%09d", i)))
		d.NodeLabels.Heap = append(d.NodeLabels.Heap, sid("L"))
		d.NodeLabelOffsets.Heap = append(d.NodeLabelOffsets.Heap, uint64(i+1))
		for _, v := range values {
			payload := testutil.Payload(v)
			if s, ok := v.Text(); ok {
				payload = uint64(sid(s))
			}
			d.NodeProps.Heap = append(d.NodeProps.Heap, graphdata.Property{Key: sid("payload"), Kind: uint8(v.Kind()), Payload: payload})
		}
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, uint64(len(d.NodeProps.Heap)))
		d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, sid(fmt.Sprintf("e%09d", i)))
		d.Sources.Heap = append(d.Sources.Heap, uint32(i))
		d.Targets.Heap = append(d.Targets.Heap, uint32((i+1)%n))
		d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, sid("L"))
		for j, v := range values {
			payload := testutil.Payload(v)
			if s, ok := v.Text(); ok {
				payload = uint64(sid(s))
			}
			d.EdgeProps.Heap = append(d.EdgeProps.Heap, graphdata.Property{Key: sid(fmt.Sprintf("p%09d", j)), Kind: uint8(v.Kind()), Payload: payload})
		}
		d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, uint64(len(d.EdgeProps.Heap)))
	}
	for i := 0; i < n; i++ {
		ps := d.NodeProps.Heap[d.NodePropOffsets.Heap[i]:d.NodePropOffsets.Heap[i+1]]
		slices.SortFunc(ps, func(a, b graphdata.Property) int {
			if graphdata.LessProperty(a, b) {
				return -1
			}
			if graphdata.LessProperty(b, a) {
				return 1
			}
			return 0
		})
	}
	graphdata.BuildCSR(d)
	if e := graphdata.BuildIndexes(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	g, e := graph.New(d, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := g.Close(); e != nil {
			t.Error(e)
		}
	})
	s, e := query.Territory(context.Background(), g, 0, query.Options{})
	if e != nil {
		t.Fatal(e)
	}
	return g, s
}
func TestExtremes(t *testing.T) {
	vs := []graph.Value{graph.BoolValue(false), graph.BoolValue(true)}
	for k := graph.ByteKind; k <= graph.LongKind; k++ {
		for _, n := range []int64{math.MinInt64, math.MinInt32, math.MinInt16, math.MinInt8, 0, math.MaxInt8, math.MaxInt16, math.MaxInt32, math.MaxInt64} {
			if v, e := graph.IntegerValue(k, n); e == nil {
				vs = append(vs, v)
			}
		}
	}
	for _, k := range []graph.ValueKind{graph.FloatKind, graph.DoubleKind} {
		for _, n := range []float64{0, math.Copysign(0, -1), 0.1, math.SmallestNonzeroFloat32, math.SmallestNonzeroFloat64, math.MaxFloat32, math.MaxFloat64, math.NaN(), math.Inf(1), math.Inf(-1)} {
			v, e := graph.DecimalValue(k, n)
			if e != nil {
				t.Fatal(e)
			}
			vs = append(vs, v)
		}
	}
	for _, k := range []graph.ValueKind{graph.StringKind, graph.DateKind, graph.DatetimeKind} {
		for _, s := range []string{"", "\"\\\n\r\t\x00<>&Olá😀", "2026-10-06T10:12:45-03:00"} {
			v, e := graph.TextValue(k, s)
			if e != nil {
				t.Fatal(e)
			}
			vs = append(vs, v)
		}
	}
	// Canonical graph property ordering, including set values and signed zero.
	slices.SortFunc(vs, func(a, b graph.Value) int {
		if a.Kind() != b.Kind() {
			return int(a.Kind()) - int(b.Kind())
		}
		if x, y := testutil.Payload(a), testutil.Payload(b); x != y {
			if x < y {
				return -1
			}
			return 1
		}
		x, _ := a.Text()
		y, _ := b.Text()
		return strings.Compare(x, y)
	})
	vs = slices.CompactFunc(vs, func(a, b graph.Value) bool { return a.Equal(b) })
	g, s := makeGraph(t, 1, vs)
	var want, wire, got bytes.Buffer
	if e := graphjson.Write(context.Background(), &want, g, s, graphjson.Query{Name: "extremes"}); e != nil {
		t.Fatal(e)
	}
	if e := Write(context.Background(), &wire, g, s, Query{Name: "extremes"}); e != nil {
		t.Fatal(e)
	}
	if e := JSON(context.Background(), &got, &wire); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Fatal("precision parity", want.String(), got.String())
	}
}
func TestLargeEntityParts(t *testing.T) {
	vs := make([]graph.Value, 30000)
	for i := range vs {
		v, e := graph.IntegerValue(graph.LongKind, int64(i))
		if e != nil {
			t.Fatal(e)
		}
		vs[i] = v
	}
	g, s := makeGraph(t, 1, vs)
	parts := 0
	maxSize := 0
	var wire, got, want bytes.Buffer
	if e := Emit(context.Background(), g, s, Query{Name: "set"}, Options{}, func(b *pb.Batch) error {
		if r := b.GetRecords(); r != nil {
			parts += len(r.Parts)
		}
		maxSize = max(maxSize, proto.Size(b))
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if parts < 10 || maxSize > MaxFrame {
		t.Fatal(parts, maxSize)
	}
	if e := Write(context.Background(), &wire, g, s, Query{Name: "set"}); e != nil {
		t.Fatal(e)
	}
	if e := JSON(context.Background(), &got, &wire); e != nil {
		t.Fatal(e)
	}
	if e := graphjson.Write(context.Background(), &want, g, s, graphjson.Query{Name: "set"}); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Fatal("continued entity parity")
	}
}

type heapWriter struct {
	baseline, peak, bytes, next uint64
	writes                      int
}

func (w *heapWriter) Write(b []byte) (int, error) {
	if w.writes == 0 || w.bytes >= w.next {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		w.peak = max(w.peak, m.HeapAlloc)
		w.next = w.bytes + 16<<20
	}
	w.bytes += uint64(len(b))
	w.writes++
	return len(b), nil
}
func TestBoundedMemory(t *testing.T) {
	v, _ := graph.TextValue(graph.StringKind, strings.Repeat("x", 1024))
	for _, n := range []int{1000, 100000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			g, s := makeGraph(t, n, []graph.Value{v})
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			e := Write(context.Background(), &failWriter{}, g, s, Query{})
			runtime.ReadMemStats(&after)
			if e != outputError || after.TotalAlloc-before.TotalAlloc > 256<<10 {
				t.Fatal("before first write", e, after.TotalAlloc-before.TotalAlloc)
			}
			runtime.GC()
			runtime.ReadMemStats(&before)
			w := &heapWriter{baseline: before.HeapAlloc}
			if e = Write(context.Background(), w, g, s, Query{}); e != nil {
				t.Fatal(e)
			}
			runtime.KeepAlive(g)
			runtime.KeepAlive(s)
			extra := uint64(0)
			if w.peak > w.baseline {
				extra = w.peak - w.baseline
			}
			if extra > 8<<20 || w.bytes < uint64(n)*2048 || w.writes < 10 {
				t.Fatal(extra, w.bytes, w.writes)
			}
			t.Logf("n=%d output=%d sampled auxiliary live heap=%d", n, w.bytes, extra)
		})
	}
}

func TestLabelContinuations(t *testing.T) {
	d := graphdata.Empty()
	d.Strings = []string{"", "node"}
	for i := 0; i < 3000; i++ {
		d.Strings = append(d.Strings, fmt.Sprintf("label%09d%s", i, strings.Repeat("x", 32)))
	}
	slices.Sort(d.Strings)
	for i, s := range d.Strings {
		if strings.HasPrefix(s, "label") {
			d.NodeLabels.Heap = append(d.NodeLabels.Heap, uint32(i))
		}
	}
	i, _ := slices.BinarySearch(d.Strings, "node")
	d.NodeIDs.Heap = []uint32{uint32(i)}
	d.NodeLabelOffsets.Heap = []uint64{0, uint64(len(d.NodeLabels.Heap))}
	d.NodePropOffsets.Heap = []uint64{0, 0}
	graphdata.BuildCSR(d)
	if e := graphdata.BuildIndexes(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	g, e := graph.New(d, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	s, e := query.NewSubgraph(g)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AddNode(0); e != nil {
		t.Fatal(e)
	}
	var want, b, got bytes.Buffer
	if e = graphjson.Write(context.Background(), &want, g, s, graphjson.Query{}); e != nil {
		t.Fatal(e)
	}
	if e = Write(context.Background(), &b, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	if e = JSON(context.Background(), &got, &b); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Fatal("labels continuation parity")
	}
}
