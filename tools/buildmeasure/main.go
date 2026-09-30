// Command buildmeasure measures the builder in an isolated process.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"
)

type discard struct{}

func (discard) Emit(context.Context, ports.Diagnostic) error { return nil }
func main() {
	root := flag.String("input", "", "generated corpus directory")
	profile := flag.String("profile", "", "optional heap profile at sampled peaks (adds overhead)")
	budget := flag.Uint64("memory-budget", 0, "external sort budget; zero measures legacy heap path")
	scratch := flag.String("temp-dir", "", "scratch directory (default: input directory)")
	flag.Parse()
	if err := run(*root, *profile, *budget, *scratch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(root, profile string, budget uint64, scratch string) error {
	n, err := filesystem.New(filepath.Join(root, "nodes"))
	if err != nil {
		return err
	}
	e, err := filesystem.New(filepath.Join(root, "edges"))
	if err != nil {
		return err
	}
	var peak, total uint64
	var phase atomic.Int32
	var phasePeaks [2]uint64
	var live uint64
	var stats scratchStats
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(10 * time.Millisecond)
		defer t.Stop()
		var lastProfile uint64
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			peak = max(peak, m.HeapAlloc)
			phasePeaks[phase.Load()] = max(phasePeaks[phase.Load()], m.HeapAlloc)
			total = m.TotalAlloc
			if profile != "" && peak > lastProfile+(128<<20) {
				f, er := os.Create(profile)
				if er == nil {
					_ = pprof.WriteHeapProfile(f)
					_ = f.Close()
				}
				lastProfile = peak
			}
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()
	options := ingest.Options{IndexProperties: []string{"group", "score"}}
	if budget > 0 {
		if scratch == "" {
			scratch = root
		}
		options.Scratch = measuredScratch{Scratch: filesystem.Scratch{Dir: scratch}, stats: &stats}
		options.MemoryBudget = budget
	}
	start := time.Now()
	g, report, err := ingest.Build(context.Background(), n, e, neptune.Decoder{}, discard{}, options)
	buildTime := time.Since(start)
	phase.Store(1)
	var writeTime time.Duration
	if err == nil {
		start = time.Now()
		_, err = snapshot.Write(context.Background(), g, file.New(filepath.Join(root, "graph.snapshot")))
		writeTime = time.Since(start)
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		live = m.HeapAlloc
		if ce := g.Close(); err == nil {
			err = ce
		}
	}
	close(done)
	wg.Wait()
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(root, "graph.snapshot"))
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Report                              ingest.Report
		Build, Write                        time.Duration
		PeakHeap, TotalAlloc, LiveGraphHeap uint64
		PhasePeakHeap                       [2]uint64
		Scratch                             scratchStats
		Snapshot                            int64
	}{report, buildTime, writeTime, peak, total, live, phasePeaks, stats, info.Size()})
}
