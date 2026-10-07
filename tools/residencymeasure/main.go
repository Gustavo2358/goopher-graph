//go:build linux && amd64 && residencybench

// residencymeasure measures native traversal after controlled file-cache/PTE
// preparation. It operates only on its own synced copy in a temporary directory.
// No global caches, production mappings or server code are changed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/snapshot/ports"
)

type source struct {
	inner ports.Source
	back  ports.Backing
}

func (s *source) Acquire(ctx context.Context) (ports.Backing, error) {
	b, err := s.inner.Acquire(ctx)
	s.back = b
	return b, err
}

type pages struct {
	Pages, ResidentPages, Size uint64
}

func observe(ctx context.Context, python, probe, path string) (pages, error) {
	out, err := exec.CommandContext(ctx, python, probe, path).Output()
	var p struct {
		Pages         uint64 `json:"pages"`
		ResidentPages uint64 `json:"resident_pages"`
		Size          uint64 `json:"size"`
	}
	if err != nil {
		return pages{}, err
	}
	err = json.Unmarshal(out, &p)
	return pages{p.Pages, p.ResidentPages, p.Size}, err
}

type phase struct {
	Millis, CPUMillis float64
	Minor, Major      int64
}

func usage() syscall.Rusage {
	var u syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &u)
	return u
}

func since(t time.Time, u syscall.Rusage) phase {
	elapsed := time.Since(t)
	v := usage()
	cpu := float64(v.Utime.Sec+v.Stime.Sec-u.Utime.Sec-u.Stime.Sec)*1000 + float64(v.Utime.Usec+v.Stime.Usec-u.Utime.Usec-u.Stime.Usec)/1000
	return phase{float64(elapsed) / float64(time.Millisecond), cpu, v.Minflt - u.Minflt, v.Majflt - u.Majflt}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	path := flag.String("snapshot", "", "source snapshot (never evicted or modified)")
	probe := flag.String("probe", "tools/residency_probe.py", "mincore observer")
	python := flag.String("python", "python3", "Python with stdlib ctypes")
	repeats := flag.Int("repeats", 20, "rotating trials per condition/query")
	encodeRepeats := flag.Int("encode-repeats", 5, "first trials additionally encode, excluded from query timing")
	flag.Parse()
	if *path == "" || *repeats < 1 || *encodeRepeats < 0 || *encodeRepeats > *repeats {
		return errors.New("invalid benchmark options")
	}
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "gophergraph-residency-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	private := filepath.Join(dir, "graph.snapshot")
	if err = copySnapshot(*path, private); err != nil {
		return err
	}
	f, err := os.Open(private)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	resident, err := mmap.NewResident(private, mmap.Warm)
	if err != nil {
		return err
	}
	src := &source{inner: resident}
	// Cold opening is measured separately: validation warms the graph itself.
	if err = mmap.BenchmarkDropFileCache(int(f.Fd())); err != nil {
		return err
	}
	beforeOpen, err := observe(ctx, *python, *probe, private)
	if err != nil || beforeOpen.ResidentPages != 0 {
		return fmt.Errorf("cold startup not established: %+v: %w", beforeOpen, err)
	}
	t, u := time.Now(), usage()
	g, err := snapshot.Open(ctx, src)
	if err != nil {
		return err
	}
	open := since(t, u)
	defer func() { err = errors.Join(err, g.Close()) }()
	afterOpen, err := observe(ctx, *python, *probe, private)
	if err != nil {
		return err
	}
	t, u = time.Now(), usage()
	report, err := resident.Prepare(ctx)
	if err != nil {
		return err
	}
	prepare := since(t, u)
	origin, err := g.FindNode("n000000000")
	if err != nil {
		return err
	}
	target, err := g.FindNode("n000000010")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	if err = enc.Encode(map[string]any{"type": "startup", "snapshot": *path, "snapshot_id": report.SnapshotID, "before_open": beforeOpen, "after_open": afterOpen, "open": open, "warm_sha_prepare": prepare, "gomaxprocs": runtime.GOMAXPROCS(0)}); err != nil {
		return err
	}
	data := src.back.Bytes()
	conditions := []string{"cold", "warm-cache", "warm"}
	if e := mmap.BenchmarkLock(data); e == nil {
		conditions = append(conditions, "locked")
		if err = mmap.BenchmarkUnlock(data); err != nil {
			return err
		}
	} else {
		if err = enc.Encode(map[string]any{"type": "unsupported", "condition": "locked", "error": e.Error(), "snapshot": *path}); err != nil {
			return err
		}
	}
	names := []string{"territory", "anti-territory", "between"}
	queryFn := func(name string) (*query.Subgraph, error) {
		switch name {
		case "territory":
			return query.Territory(ctx, g, origin, query.Options{})
		case "anti-territory":
			return query.AntiTerritory(ctx, g, origin, query.Options{})
		default:
			return query.Between(ctx, g, origin, target, query.Options{})
		}
	}
	// Warm Go code/workspaces before preparing the controlled snapshot state.
	for _, name := range names {
		if _, err = queryFn(name); err != nil {
			return err
		}
	}
	for trial := range *repeats {
		for q := range names {
			name := names[(q+trial)%len(names)]
			for c := range conditions {
				condition := conditions[(c+trial)%len(conditions)]
				runtime.GC()
				if err = mmap.BenchmarkPrepare(data, int(f.Fd()), condition); err != nil {
					return err
				}
				if err = measureTrial(ctx, enc, *python, *probe, private, name, condition, trial, trial < *encodeRepeats, queryFn, g); condition == "locked" {
					err = errors.Join(err, mmap.BenchmarkUnlock(data))
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copySnapshot(from, to string) (err error) {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

func measureTrial(ctx context.Context, enc *json.Encoder, python, probe, path, name, condition string, trial int, encode bool, queryFn func(string) (*query.Subgraph, error), g *graph.Graph) error {
	before, err := observe(ctx, python, probe, path)
	if err != nil {
		return err
	}
	if (condition == "cold" && before.ResidentPages != 0) || (condition != "cold" && before.ResidentPages != before.Pages) {
		return fmt.Errorf("condition %s not established: %+v", condition, before)
	}
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	t, u := time.Now(), usage()
	sub, err := queryFn(name)
	queryPhase := since(t, u)
	runtime.ReadMemStats(&b)
	if err != nil {
		return err
	}
	nodes, edges, err := sub.CountsContext(ctx)
	if err != nil {
		return err
	}
	afterQuery, err := observe(ctx, python, probe, path)
	if err != nil {
		return err
	}
	var encoding *phase
	var payload uint64
	if encode {
		t, u = time.Now(), usage()
		err = ggpb.EmitEncoded(ctx, g, sub, ggpb.Query{Name: name}, ggpb.Options{}, func(p []byte) error { payload += uint64(len(p)); return nil })
		v := since(t, u)
		encoding = &v
		if err != nil {
			return err
		}
	}
	return enc.Encode(map[string]any{"type": "trial", "query": name, "condition": condition, "trial": trial, "before": before, "after_query": afterQuery, "query_phase": queryPhase, "encode_phase": encoding, "nodes": nodes, "edges": edges, "payload_bytes": payload, "query_alloc_bytes": b.TotalAlloc - a.TotalAlloc, "query_allocations": b.Mallocs - a.Mallocs})
}
