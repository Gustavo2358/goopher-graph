package ggpb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"hash/crc32"
	"io"
	"os"
	"runtime"

	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// Measured prototype, not a product/default option. Cloning owns each generated
// batch beyond Emit's callback. A window bounds jobs and ordered completions.
func parallelWrite(ctx context.Context, out io.Writer, g *graph.Graph, s *query.Subgraph, q Query, workers int) error {
	if workers == 1 {
		return Write(ctx, out, g, s, q)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type job struct {
		n int
		b *pb.Batch
	}
	type result struct {
		n   int
		raw []byte
	}
	jobs := make(chan job, workers)
	ready := make(chan result, workers)
	window := make(chan struct{}, workers*2)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			var w wireEncoder
			for j := range jobs {
				raw := bytes.Clone(w.batch(j.b))
				select {
				case ready <- result{j.n, raw}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	done := make(chan error, 1)
	go func() {
		pending := make(map[int][]byte)
		next := 0
		first := true
		for r := range ready {
			pending[r.n] = r.raw
			for {
				raw, ok := pending[next]
				if !ok {
					break
				}
				delete(pending, next)
				next++
				if first {
					if e := writeAll(out, magic[:]); e != nil {
						cancel()
						done <- e
						return
					}
					first = false
				}
				var word [4]byte
				binary.LittleEndian.PutUint32(word[:], uint32(len(raw)))
				if e := writeAll(out, word[:]); e != nil {
					cancel()
					done <- e
					return
				}
				if e := writeAll(out, raw); e != nil {
					cancel()
					done <- e
					return
				}
				binary.LittleEndian.PutUint32(word[:], crc32.ChecksumIEEE(raw))
				if e := writeAll(out, word[:]); e != nil {
					cancel()
					done <- e
					return
				}
				<-window
			}
		}
		done <- nil
	}()
	n := 0
	e := Emit(ctx, g, s, q, Options{}, func(b *pb.Batch) error {
		select {
		case window <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		j := job{n, proto.Clone(b).(*pb.Batch)}
		n++
		select {
		case jobs <- j:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	close(jobs)
	wg.Wait()
	close(ready)
	writeErr := <-done
	if writeErr != nil {
		return writeErr
	}
	return e
}

func TestParallelPrototype(t *testing.T) {
	g, s := fixture(t, "typed")
	var want bytes.Buffer
	if e := Write(context.Background(), &want, g, s, Query{}); e != nil {
		t.Fatal(e)
	}
	for _, workers := range []int{2, 4, 8} {
		var got bytes.Buffer
		if e := parallelWrite(context.Background(), &got, g, s, Query{}, workers); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(want.Bytes(), got.Bytes()) {
			t.Fatal("parallel ordering/wire")
		}
		if e := parallelWrite(context.Background(), &failWriter{}, g, s, Query{}, workers); e != outputError {
			t.Fatal(e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if e := parallelWrite(ctx, io.Discard, g, s, Query{}, workers); e != context.Canceled {
			t.Fatal(e)
		}
	}
}
func experimentCPU() float64 {
	var r syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &r)
	return float64(r.Utime.Sec+r.Stime.Sec)*1000 + float64(r.Utime.Usec+r.Stime.Usec)/1000
}

// Explicit opt-in, excludes builds/open/query and shares one immutable snapshot.
// Each wave has N independent query encoders, bounded windows private to each.
func TestConcurrencyExperiment(t *testing.T) {
	root := os.Getenv("GGPB_EXPERIMENT_FIXTURES")
	if root == "" {
		t.Skip("explicit performance campaign only")
	}
	path := os.Getenv("GGPB_EXPERIMENT_OUTPUT")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	log := json.NewEncoder(f)
	ctx := context.Background()
	origin := "n000000000"
	for _, size := range []string{"100", "10000", "100000", "empty"} {
		dir := size
		if dir == "empty" {
			dir = "100"
		}
		g, e := snapshot.Open(ctx, mmap.New(root+"/"+dir+"/graph.snapshot"))
		if e != nil {
			t.Fatal(e)
		}
		id, e := g.FindNode(origin)
		if e != nil {
			t.Fatal(e)
		}
		sub, e := query.Territory(ctx, g, id, query.Options{})
		if e != nil {
			t.Fatal(e)
		}
		q := Query{Name: "territory", Node: &origin}
		if size == "empty" {
			to := "n000000090"
			end, err := g.FindNode(to)
			if err != nil {
				t.Fatal(err)
			}
			sub, e = query.Between(ctx, g, id, end, query.Options{})
			if e != nil {
				t.Fatal(e)
			}
			q = Query{Name: "between", From: &origin, To: &to}
		}
		if e := Write(ctx, io.Discard, g, sub, q); e != nil {
			t.Fatal(e)
		}
		for _, workers := range []int{1, 2, 4, 8} {
			for _, concurrent := range []int{1, 2, 4, 8} {
				for rep := 0; rep < 3; rep++ {
					runtime.GC()
					var a, b runtime.MemStats
					runtime.ReadMemStats(&a)
					c := experimentCPU()
					start := time.Now()
					var wg sync.WaitGroup
					encTimes := make([]float64, concurrent)
					queryTimes := make([]float64, concurrent)
					for i := range concurrent {
						wg.Go(func() {
							qt := time.Now()
							result, err := query.Territory(ctx, g, id, query.Options{})
							if size == "empty" {
								end, _ := g.FindNode("n000000090")
								result, err = query.Between(ctx, g, id, end, query.Options{})
							}
							if err != nil {
								t.Error(err)
								return
							}
							queryTimes[i] = float64(time.Since(qt)) / float64(time.Millisecond)
							et := time.Now()
							if e := parallelWrite(ctx, io.Discard, g, result, q, workers); e != nil {
								t.Error(e)
							}
							encTimes[i] = float64(time.Since(et)) / float64(time.Millisecond)
						})
					}
					wg.Wait()
					elapsed := float64(time.Since(start)) / float64(time.Millisecond)
					cpu := experimentCPU() - c
					runtime.ReadMemStats(&b)
					if e := log.Encode(map[string]any{"Case": size, "Workers": workers, "Queries": concurrent, "Repeat": rep, "WaveMS": elapsed, "AmortizedMSPerQuery": elapsed / float64(concurrent), "EncodingLatenciesMS": encTimes, "QueryLatenciesMS": queryTimes, "CPUTimeMS": cpu, "AllocBytes": b.TotalAlloc - a.TotalAlloc, "Allocs": b.Mallocs - a.Mallocs, "HeapAfter": b.HeapAlloc}); e != nil {
						t.Fatal(e)
					}
				}
			}
		}
		if e := g.Close(); e != nil {
			t.Fatal(e)
		}
	}
}

// GC samples are intentionally outside timing campaigns. Measures live auxiliary
// retention and actual frame geometry on the same four snapshots/results.
func TestBatchGeometryExperiment(t *testing.T) {
	root := os.Getenv("GGPB_GEOMETRY_FIXTURES")
	if root == "" {
		t.Skip("explicit performance campaign only")
	}
	f, e := os.Create(os.Getenv("GGPB_GEOMETRY_OUTPUT"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	log := json.NewEncoder(f)
	for _, size := range []string{"100", "10000", "100000", "empty"} {
		dir := size
		if dir == "empty" {
			dir = "100"
		}
		ctx := context.Background()
		g, e := snapshot.Open(ctx, mmap.New(root+"/"+dir+"/graph.snapshot"))
		if e != nil {
			t.Fatal(e)
		}
		id, e := g.FindNode("n000000000")
		if e != nil {
			t.Fatal(e)
		}
		s, e := query.Territory(ctx, g, id, query.Options{})
		if e != nil {
			t.Fatal(e)
		}
		if size == "empty" {
			end, _ := g.FindNode("n000000090")
			s, e = query.Between(ctx, g, id, end, query.Options{})
			if e != nil {
				t.Fatal(e)
			}
		}
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		baseline := m.HeapAlloc
		peak := baseline
		frames, count, maxSize, total := 0, 0, 0, 0
		if e := EmitEncoded(ctx, g, s, Query{}, Options{}, func(raw []byte) error {
			frames++
			maxSize = max(maxSize, len(raw))
			total += len(raw) + 8
			if raw[0] == 0x12 {
				count++
			}
			if count%64 == 0 || frames <= 2 {
				runtime.GC()
				runtime.ReadMemStats(&m)
				peak = max(peak, m.HeapAlloc)
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		runtime.KeepAlive(g)
		runtime.KeepAlive(s)
		if e := log.Encode(map[string]any{"Case": size, "TargetBytes": batchTarget, "MaxParts": maxParts, "Frames": frames, "RecordsBatches": count, "MaxFrameBytes": maxSize, "PayloadBytes": total + 8, "SampledLiveAuxBytes": peak - baseline}); e != nil {
			t.Fatal(e)
		}
		if e := g.Close(); e != nil {
			t.Fatal(e)
		}
	}
}
