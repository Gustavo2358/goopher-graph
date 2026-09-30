package ingest_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/internal/benchfixture"
	"gophergraph/query"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type ignoreDiagnostic struct{}

func (ignoreDiagnostic) Emit(context.Context, ports.Diagnostic) error { return nil }

type scaleMemory struct{ Peak, Live uint64 }

func TestExternalScaleWorker(t *testing.T) {
	dir := os.Getenv("GOPHERGRAPH_SCALE_WORKER")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	n, err := strconv.Atoi(os.Getenv("GOPHERGRAPH_SCALE_N"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	result := make(chan uint64)
	go func() {
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		var peak uint64
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			peak = max(peak, m.HeapAlloc)
			select {
			case <-done:
				result <- peak
				return
			case <-tick.C:
			}
		}
	}()
	nodes, err := filesystem.New(filepath.Join(dir, "nodes"))
	if err != nil {
		t.Fatal(err)
	}
	edges, err := filesystem.New(filepath.Join(dir, "edges"))
	if err != nil {
		t.Fatal(err)
	}
	g, r, err := ingest.Build(context.Background(), nodes, edges, neptune.Decoder{}, ignoreDiagnostic{}, ingest.Options{Scratch: filesystem.Scratch{Dir: dir}, MemoryBudget: 1 << 20, IndexProperties: []string{"group", "score"}})
	close(done)
	peak := <-result
	wantNodes, wantEdges := uint64(n), uint64(n*5)
	startID := "n000000000"
	wantReach := uint64(n * 9 / 10)
	if os.Getenv("GOPHERGRAPH_SCALE_SKEW") == "1" {
		wantNodes, wantEdges = 1, 1
		startID = "A"
		wantReach = 1
	}
	if err != nil || r.Nodes != wantNodes || r.Edges != wantEdges || r.Completeness != ingest.Complete {
		t.Fatal(r, err)
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// The completed graph must remain queryable after all scratch names are gone.
	id, err := g.FindNode(startID)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []graph.Direction{graph.Forward, graph.Reverse} {
		reached, err := query.Reachable(context.Background(), g, id, direction, query.Options{})
		if err != nil || reached.Count() != wantReach {
			t.Fatal(reached, err)
		}
	}
	if err = g.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(scaleMemory{peak, m.HeapAlloc})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "memory.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestExternalGeneratedScale(t *testing.T) {
	sizes := []int{1200, 12000}
	// Opt-in qualification generates 1.2M and 2.4M rows; the default test is small
	// enough for every regression. Files are generated incrementally, not in RAM.
	if os.Getenv("GOPHERGRAPH_SCALE") == "1" {
		sizes = []int{200000, 400000}
	}
	for _, n := range sizes {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			dir := t.TempDir()
			if err := benchfixture.Write(dir, n); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestExternalScaleWorker$", "-test.count=1")
			cmd.Env = append(os.Environ(), "GOPHERGRAPH_SCALE_WORKER="+dir, "GOPHERGRAPH_SCALE_N="+strconv.Itoa(n))
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s\n%v", b, err)
			}
			b, err := os.ReadFile(filepath.Join(dir, "memory.json"))
			if err != nil {
				t.Fatal(err)
			}
			var m scaleMemory
			if err = json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			t.Logf("%d rows: peak heap=%d live heap=%d", n*6, m.Peak, m.Live)
			// Constant bounds in isolated processes reject retaining entities, strings,
			// final columns or a graph-sized validation workspace on the Go heap.
			if m.Peak > 24<<20 || m.Live > 8<<20 {
				t.Fatalf("heap grew beyond fixed bound: %+v", m)
			}
		})
	}
}

func TestExternalSingleEntityScale(t *testing.T) {
	if os.Getenv("GOPHERGRAPH_SCALE") != "1" {
		t.Skip("set GOPHERGRAPH_SCALE=1 for large skew qualification")
	}
	dir := t.TempDir()
	for _, role := range []string{"nodes", "edges"} {
		if err := os.Mkdir(filepath.Join(dir, role), 0700); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, "nodes", "data.csv"))
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "~id,group:String")
	tail := strings.Repeat("x", 256)
	for i := 0; i < 240000; i++ {
		if _, err = fmt.Fprintf(w, "A,v%09d%s\n", i, tail); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "edges", "data.csv"), []byte("~id,~from,~to\ne,A,A\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExternalScaleWorker$", "-test.count=1")
	cmd.Env = append(os.Environ(), "GOPHERGRAPH_SCALE_WORKER="+dir, "GOPHERGRAPH_SCALE_N=1", "GOPHERGRAPH_SCALE_SKEW=1")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s\n%v", b, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m scaleMemory
	if err = json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	t.Logf("240000 set values on one ID: peak heap=%d live heap=%d", m.Peak, m.Live)
	if m.Peak > 24<<20 || m.Live > 8<<20 {
		t.Fatalf("single group retained: %+v", m)
	}
}
