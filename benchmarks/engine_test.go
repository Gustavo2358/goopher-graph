package benchmarks

import (
	"context"
	"fmt"
	"gophergraph/dot"
	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	ingestports "gophergraph/ingest/ports"
	"gophergraph/internal/benchfixture"
	"gophergraph/internal/graphdata"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type discard struct{}

func (discard) Emit(context.Context, ingestports.Diagnostic) error { return nil }
func procKB(key string) uint64 {
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == key+":" {
			n, _ := strconv.ParseUint(f[1], 10, 64)
			return n * 1024
		}
	}
	return 0
}
func checkSize(b testing.TB, g *graph.Graph, size uint64) {
	b.Helper()
	var d graphdata.Data
	if e := g.InternalColumns(&d); e != nil {
		b.Fatal(e)
	}
	n, e, s := d.NodeIDs.Len(), d.EdgeIDs.Len(), d.StringCount()
	l, pn, pe := d.NodeLabels.Len(), d.NodeProps.Len(), d.EdgeProps.Len()
	var textBytes uint64
	for i := uint64(0); i < s; i++ {
		textBytes += uint64(len(d.String(uint32(i))))
	}
	formula := 8*(s+1) + textBytes + 4*n + 16*(n+1) + 4*l + 16*pn + 16*e + 8*(e+1) + 16*pe + 16*(n+1) + 16*e + 24*d.LabelIndex.Len() + 4*(l+e) + 4*d.IndexedKeys.Len() + 32*d.PropertyIndex.Len() + 4*d.PropertyPostings.Len()
	lengths := []uint64{8 * (s + 1), textBytes, 4 * n, 8 * (n + 1), 4 * l, 8 * (n + 1), 16 * pn, 4 * e, 4 * e, 4 * e, 4 * e, 8 * (e + 1), 16 * pe, 8 * (n + 1), 4 * e, 4 * e, 8 * (n + 1), 4 * e, 4 * e, 24 * d.LabelIndex.Len(), 4 * (l + e), 4 * d.IndexedKeys.Len(), 32 * d.PropertyIndex.Len(), 4 * d.PropertyPostings.Len()}
	var raw, padding uint64
	for _, v := range lengths {
		raw += v
		padding += ((v + 63) &^ 63) - v
	}
	if raw != formula || 832+formula+padding != size {
		b.Fatalf("size formula: raw=%d formula=%d padding=%d file=%d", raw, formula, padding, size)
	}
}
func BenchmarkEngine(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("N%d_E%d", n, 5*n), func(b *testing.B) {
			ctx := context.Background()
			dir := b.TempDir()
			if e := benchfixture.Write(dir, n); e != nil {
				b.Fatal(e)
			}
			nc, e := filesystem.New(filepath.Join(dir, "nodes"))
			if e != nil {
				b.Fatal(e)
			}
			ec, e := filesystem.New(filepath.Join(dir, "edges"))
			if e != nil {
				b.Fatal(e)
			}
			build := func() (*graph.Graph, ingest.Report) {
				g, r, e := ingest.Build(ctx, nc, ec, neptune.Decoder{}, discard{}, ingest.Options{IndexProperties: []string{"group", "score"}})
				if e != nil || r.Nodes != uint64(n) || r.Edges != uint64(n*5) || r.Completeness != ingest.Complete {
					b.Fatal(r, e)
				}
				return g, r
			}
			heap, report := build()
			path := filepath.Join(dir, "graph.snapshot")
			if _, e = snapshot.Write(ctx, heap, file.New(path)); e != nil {
				b.Fatal(e)
			}
			info, e := os.Stat(path)
			if e != nil {
				b.Fatal(e)
			}
			checkSize(b, heap, uint64(info.Size()))
			_ = heap.Close()
			runtime.GC()
			g, e := snapshot.Open(ctx, mmap.New(path))
			if e != nil {
				b.Fatal(e)
			}
			defer g.Close()
			start, e := g.FindNode("n000000000")
			if e != nil {
				b.Fatal(e)
			}
			sub, e := query.Territory(ctx, g, start, query.Options{})
			if e != nil || sub.NodeCount() != uint64(n*9/10) || sub.EdgeCount() != uint64(n*9/10*5) {
				b.Fatal(e, sub.NodeCount(), sub.EdgeCount())
			}
			var samples []float64
			for i := 0; i < 7; i++ {
				t := time.Now()
				s, e := query.Reachable(ctx, g, start, graph.Forward, query.Options{})
				if e != nil || s.Count() != uint64(n*9/10) {
					b.Fatal(e)
				}
				samples = append(samples, float64(time.Since(t).Nanoseconds()))
			}
			sort.Float64s(samples)
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			b.Logf("snapshot=%d B ingest+merge=%s canonicalize+CSR+indexes=%s; 7 warm queries median=%.0f ns min=%.0f max=%.0f; mapped=%d B heap=%d B RSS=%d B process_peak_RSS=%d B", info.Size(), report.Times.IngestMerge, report.Times.Canonicalize, samples[3], samples[0], samples[6], info.Size(), mem.HeapAlloc, procKB("VmRSS"), procKB("VmHWM"))
			b.Run("Build", func(b *testing.B) {
				b.ReportAllocs()
				var ingestTime, canonicalTime time.Duration
				for i := 0; i < b.N; i++ {
					g, r := build()
					ingestTime += r.Times.IngestMerge
					canonicalTime += r.Times.Canonicalize
					_ = g.Close()
				}
				b.ReportMetric(float64(ingestTime.Nanoseconds())/float64(b.N), "ingest-merge-ns/op")
				b.ReportMetric(float64(canonicalTime.Nanoseconds())/float64(b.N), "canonical-ns/op")
			})
			b.Run("WriteValidateCommit", func(b *testing.B) {
				b.ReportAllocs()
				sink := file.New(filepath.Join(dir, "copy.snapshot"))
				for i := 0; i < b.N; i++ {
					if _, e := snapshot.Write(ctx, g, sink); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("OpenValidate", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					v, e := snapshot.Open(ctx, mmap.New(path))
					if e != nil {
						b.Fatal(e)
					}
					if e = v.Close(); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("Reachable", func(b *testing.B) {
				b.ReportMetric(samples[3], "warm-median-ns")
				b.ReportMetric(samples[0], "warm-min-ns")
				b.ReportMetric(samples[6], "warm-max-ns")
				b.ReportMetric(float64(info.Size()), "mapped-B")
				b.ReportMetric(float64(mem.HeapAlloc), "heap-B")
				b.ReportMetric(float64(procKB("VmRSS")), "rss-B")
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, e := query.Reachable(ctx, g, start, graph.Forward, query.Options{}); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("Subgraph", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, e := query.Territory(ctx, g, start, query.Options{}); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("JSON", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if e := graphjson.Write(ctx, io.Discard, g, sub, graphjson.Query{Name: "territory", Node: new("n000000000")}); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("GGPB", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if e := ggpb.Write(ctx, io.Discard, g, sub, ggpb.Query{Name: "territory", Node: new("n000000000")}); e != nil {
						b.Fatal(e)
					}
				}
			})
			b.Run("DOT", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if e := dot.Write(io.Discard, g, sub); e != nil {
						b.Fatal(e)
					}
				}
			})
		})
	}
}
