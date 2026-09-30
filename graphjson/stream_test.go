package graphjson_test

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/internal/graphdata"
	"gophergraph/query"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The input is compact: one shared 1 KiB property value and a directed cycle.
// Query construction and graph validation happen before serialization accounting.
func largeSubgraph(t testing.TB, n int) (*graph.Graph, *query.Subgraph) {
	t.Helper()
	d := graphdata.Empty()
	payload := strings.Repeat("x", 1024)
	d.Strings = []string{"", "L", "payload", payload}
	for i := 0; i < n; i++ {
		d.Strings = append(d.Strings, fmt.Sprintf("e%09d", i), fmt.Sprintf("n%09d", i))
	}
	slices.Sort(d.Strings)
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
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, uint64(i+1))
		d.NodeProps.Heap = append(d.NodeProps.Heap, graphdata.Property{Key: sid("payload"), Kind: uint8(graph.StringKind), Payload: uint64(sid(payload))})
		d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, sid(fmt.Sprintf("e%09d", i)))
		d.Sources.Heap = append(d.Sources.Heap, uint32(i))
		d.Targets.Heap = append(d.Targets.Heap, uint32((i+1)%n))
		d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, sid("L"))
		d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, uint64(i+1))
		d.EdgeProps.Heap = append(d.EdgeProps.Heap, d.NodeProps.Heap[i])
	}
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	g, err := graph.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	sub, err := query.Territory(context.Background(), g, 0, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return g, sub
}

type measuredWriter struct {
	bytes, writes, nextSample uint64
	peak, baseline            uint64
	cancel                    context.CancelFunc
}

func (w *measuredWriter) Write(p []byte) (int, error) {
	if w.writes == 0 || w.bytes >= w.nextSample {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		if m.HeapAlloc > w.peak {
			w.peak = m.HeapAlloc
		}
		w.nextSample = w.bytes + (16 << 20)
	}
	w.bytes += uint64(len(p))
	w.writes++
	if w.cancel != nil {
		w.cancel()
	}
	return len(p), nil
}
func TestStreamingLargeResult(t *testing.T) {
	for _, n := range []int{1000, 100000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			g, sub := largeSubgraph(t, n)
			q := graphjson.Query{Name: "territory", Node: new("n000000000")}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			// A whole-result DTO or JSON buffer before the first write fails this gate,
			// even if it would be collected later. No reader/output buffer is retained.
			err := graphjson.Write(context.Background(), failingWriter{}, g, sub, q)
			runtime.ReadMemStats(&after)
			if !errors.Is(err, errOutput) || after.TotalAlloc-before.TotalAlloc > 256<<10 {
				t.Fatalf("before first write: alloc=%d err=%v", after.TotalAlloc-before.TotalAlloc, err)
			}
			runtime.GC()
			runtime.ReadMemStats(&before)
			w := &measuredWriter{baseline: before.HeapAlloc}
			if err := graphjson.Write(context.Background(), w, g, sub, q); err != nil {
				t.Fatal(err)
			}
			runtime.KeepAlive(g)
			runtime.KeepAlive(sub)
			extra := uint64(0)
			if w.peak > w.baseline {
				extra = w.peak - w.baseline
			}
			// Same fixed allowance at both scales; catches materializing entities after
			// a token header write as well as retaining the complete serialized JSON.
			if extra > 8<<20 || w.bytes < uint64(n)*2048 || w.writes < 100 {
				t.Fatalf("heap=%d output=%d writes=%d", extra, w.bytes, w.writes)
			}
			t.Logf("nodes=%d edges=%d output=%d bytes sampled live heap delta=%d bytes", n, n, w.bytes, extra)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			canceled := &measuredWriter{cancel: cancel}
			if err := graphjson.Write(ctx, canceled, g, sub, q); !errors.Is(err, context.Canceled) || canceled.writes != 1 {
				t.Fatalf("cancel: %v writes=%d", err, canceled.writes)
			}
		})
	}
}
