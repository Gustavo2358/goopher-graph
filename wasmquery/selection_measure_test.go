package wasmquery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/query"
	"gophergraph/snapshot"
	snapshotfile "gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/adapters/mmap"
)

type selectionSample struct {
	MappedBytes                                  uint64
	Phase, Mode                                  string
	Indexed                                      bool
	Sample                                       int
	Nodes, Edges, Candidates, OutputNodes, Bytes uint64
	Millis                                       float64
	AllocBytes, Allocs                           uint64
	HostCalls, PeakHostBytes                     uint64
	PeakRSSKiB                                   int64
}

// Opt-in measurement, intentionally separate from correctness gates. Compiling,
// building and opening are excluded from query samples and recorded separately.
func TestSelectionMeasurement(t *testing.T) {
	output := os.Getenv("GOPHERGRAPH_SELECTION_MEASURE")
	if output == "" {
		t.Skip("set GOPHERGRAPH_SELECTION_MEASURE to a JSONL output path")
	}
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	var mappedBytes uint64
	record := func(s selectionSample) {
		s.MappedBytes = mappedBytes
		var usage syscall.Rusage
		_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
		s.PeakRSSKiB = usage.Maxrss
		if err := encoder.Encode(s); err != nil {
			t.Fatal(err)
		}
	}
	const n = 100000
	const warmup = 3
	const samples = 11
	root := t.TempDir()
	for _, role := range []string{"nodes", "edges"} {
		if err := os.Mkdir(filepath.Join(root, role), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(role, header string, body func(*bufio.Writer)) {
		f, err := os.Create(filepath.Join(root, role, "data.csv"))
		if err != nil {
			t.Fatal(err)
		}
		w := bufio.NewWriter(f)
		if _, err = fmt.Fprintln(w, header); err != nil {
			t.Fatal(err)
		}
		body(w)
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write("nodes", "~id,~label,tag:String,rank:Int", func(w *bufio.Writer) {
		for i := 0; i < n; i++ {
			label := "OTHER"
			if i%100 < 6 {
				label = fmt.Sprintf("L%d", i%100)
			}
			if i%100 == 0 {
				label = "L0;L1"
			}
			_, err := fmt.Fprintf(w, "n%09d,%s,V%d,%d\n", i, label, i%7, i)
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	write("edges", "~id,~from,~to,~label", func(w *bufio.Writer) {
		for i := 0; i < n; i++ {
			if _, err := fmt.Fprintf(w, "e%09d,n%09d,n%09d,LINK\n", i, i, (i+1)%n); err != nil {
				t.Fatal(err)
			}
		}
	})
	ctx := context.Background()
	labels := []string{"L0", "L1", "L2", "L3", "L4", "L5"}
	value, _ := graph.TextValue(graph.StringKind, "V0")
	expected := uint64(0)
	for i := 0; i < n; i++ {
		if i%100 < 6 && i%7 == 0 {
			expected++
		}
	}
	r := newRuntime(t, Limits{})
	start := time.Now()
	guest := buildGuest(t, "./testdata/guest")
	record(selectionSample{Phase: "guest-build", Millis: float64(time.Since(start)) / 1e6})
	start = time.Now()
	m, err := r.Compile(ctx, guest)
	if err != nil {
		t.Fatal(err)
	}
	record(selectionSample{Phase: "module-compile", Millis: float64(time.Since(start)) / 1e6})
	labelJSON, _ := json.Marshal(labels)
	valueJSON, _ := json.Marshal(encodeValue(value))
	for _, indexed := range []bool{false, true} {
		start = time.Now()
		built := loadGraph(t, root, indexed, "tag")
		record(selectionSample{Phase: "build", Indexed: indexed, Millis: float64(time.Since(start)) / 1e6})
		path := filepath.Join(root, fmt.Sprintf("%v.snapshot", indexed))
		start = time.Now()
		if _, err := snapshot.Write(ctx, built, snapshotfile.New(path)); err != nil {
			t.Fatal(err)
		}
		record(selectionSample{Phase: "write", Indexed: indexed, Millis: float64(time.Since(start)) / 1e6})
		built.Close()
		start = time.Now()
		g, err := snapshot.Open(ctx, mmap.New(path))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		mappedBytes += uint64(info.Size())
		record(selectionSample{Phase: "open", Indexed: indexed, Millis: float64(time.Since(start)) / 1e6})
		defer g.Close()
		old := func() (*graph.NodeSet, error) {
			result, err := graph.NewNodeSet(g)
			if err != nil {
				return nil, err
			}
			for _, label := range labels {
				candidates, err := g.NodesWithLabel(ctx, label)
				if err != nil {
					return nil, err
				}
				global, err := g.NodesWithProperty(ctx, "tag", value)
				if err != nil {
					return nil, err
				}
				filtered, err := candidates.IntersectionContext(ctx, global)
				if err != nil {
					return nil, err
				}
				result, err = result.UnionContext(ctx, filtered)
				if err != nil {
					return nil, err
				}
			}
			return result, nil
		}
		composed := func() (*graph.NodeSet, error) { return g.NodesWithAnyLabelAndProperty(ctx, labels, "tag", value) }
		check := func(nodes *graph.NodeSet) {
			if nodes.Count() != expected {
				t.Fatal(nodes.Count(), expected)
			}
			it := nodes.Iterator()
			for it.Next() {
				id := uint64(it.ID())
				if id%100 >= 6 || id%7 != 0 {
					t.Fatal("unexpected member", id)
				}
			}
		}
		for _, mode := range []string{"core-global-per-label", "core-composed", "wasm-label-has", "wasm-composed"} {
			for i := -warmup; i < samples; i++ {
				var before, after runtime.MemStats
				if i == 0 {
					runtime.GC()
				}
				runtime.ReadMemStats(&before)
				start = time.Now()
				var report Metrics
				var result *Result
				if mode == "core-global-per-label" || mode == "core-composed" {
					var nodes *graph.NodeSet
					if mode == "core-global-per-label" {
						nodes, err = old()
					} else {
						nodes, err = composed()
					}
					elapsed := time.Since(start)
					runtime.ReadMemStats(&after)
					if err != nil {
						t.Fatal(err)
					}
					check(nodes)
					if i >= 0 {
						record(selectionSample{Phase: "selection", Mode: mode, Indexed: indexed, Sample: i, Nodes: n, Edges: n, Candidates: 6000, OutputNodes: expected, Millis: float64(elapsed) / 1e6, AllocBytes: after.TotalAlloc - before.TotalAlloc, Allocs: after.Mallocs - before.Mallocs})
					}
				} else {
					strategy := "previous"
					if mode == "wasm-composed" {
						strategy = "composed"
					}
					result, report, err = r.ExecuteReport(ctx, m, g, []string{"selection", strategy, string(labelJSON), "tag", string(valueJSON)})
					elapsed := time.Since(start)
					runtime.ReadMemStats(&after)
					if err != nil {
						t.Fatal(err)
					}
					sub, err := result.Subgraph()
					if err != nil || sub.NodeCount() != expected {
						t.Fatal(err)
					}
					result.Close()
					if i >= 0 {
						record(selectionSample{Phase: "execution+materialization", Mode: mode, Indexed: indexed, Sample: i, Nodes: n, Edges: n, Candidates: 6000, OutputNodes: expected, Millis: float64(elapsed) / 1e6, AllocBytes: after.TotalAlloc - before.TotalAlloc, Allocs: after.Mallocs - before.Mallocs, HostCalls: report.HostCalls, PeakHostBytes: report.PeakHostBytes})
					}
				}
			}
		}
		nodes, err := composed()
		if err != nil {
			t.Fatal(err)
		}
		var sub *query.Subgraph
		var encoded bytes.Buffer
		for i := -warmup; i < samples; i++ {
			start = time.Now()
			sub, err = query.FromNodes(ctx, g, nodes, query.Options{})
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if i >= 0 {
				record(selectionSample{Phase: "materialization", Indexed: indexed, Sample: i, OutputNodes: expected, Millis: float64(elapsed) / 1e6})
			}
			for _, format := range []string{"graphjson", "ggpb-file"} {
				encoded.Reset()
				start = time.Now()
				if format == "graphjson" {
					err = graphjson.Write(ctx, &encoded, g, sub, graphjson.Query{Name: "selection"})
				} else {
					err = ggpb.Write(ctx, &encoded, g, sub, ggpb.Query{Name: "selection"})
				}
				elapsed = time.Since(start)
				if err != nil {
					t.Fatal(err)
				}
				if i >= 0 {
					record(selectionSample{Phase: "serialization", Mode: format, Indexed: indexed, Sample: i, OutputNodes: expected, Bytes: uint64(encoded.Len()), Millis: float64(elapsed) / 1e6})
				}
				if format == "ggpb-file" {
					start = time.Now()
					if err := ggpb.JSON(ctx, io.Discard, bytes.NewReader(encoded.Bytes())); err != nil {
						t.Fatal(err)
					}
					elapsed = time.Since(start)
					if i >= 0 {
						record(selectionSample{Phase: "conversion", Mode: "ggpb-file-to-json", Indexed: indexed, Sample: i, OutputNodes: expected, Bytes: uint64(encoded.Len()), Millis: float64(elapsed) / 1e6})
					}
				}
			}
		}
		assertClean(t, r)
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		mappedBytes -= uint64(info.Size())
	}
}
