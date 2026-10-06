// Command encodingmeasure profiles encoding on an opened snapshot/result.
// Query and opening costs are excluded. Build before measuring.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gophergraph/ggpb"
	"gophergraph/graphjson"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func cpu() float64 {
	var r syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &r)
	return float64(r.Utime.Sec+r.Stime.Sec)*1000 + float64(r.Utime.Usec+r.Stime.Usec)/1000
}
func run() error {
	path := flag.String("snapshot", "", "snapshot")
	format := flag.String("format", "ggpb", "json, ggpb, ggpb-inline")
	output := flag.String("output", "", "file, otherwise discard")
	cp := flag.String("cpuprofile", "", "CPU profile")
	mp := flag.String("memprofile", "", "allocation/heap profile")
	repeats := flag.Int("repeats", 5, "resident encodings")
	warm := flag.Int("warmup", 1, "unmeasured encodings")
	kind := flag.String("query", "territory", "territory, anti-territory or between")
	queryEach := flag.Bool("query-each", false, "run a new query before each resident encoding")
	empty := flag.Bool("empty", false, "between disconnected components")
	flag.Parse()
	ctx := context.Background()
	g, e := snapshot.Open(ctx, mmap.New(*path))
	if e != nil {
		return e
	}
	defer g.Close()
	id, e := g.FindNode("n000000000")
	if e != nil {
		return e
	}
	origin := "n000000000"
	target := "n000000010"
	if *empty {
		target = "n000000090"
		*kind = "between"
	}
	end, e := g.FindNode(target)
	if e != nil {
		return e
	}
	runQuery := func() (*query.Subgraph, error) {
		switch *kind {
		case "territory":
			return query.Territory(ctx, g, id, query.Options{})
		case "anti-territory":
			return query.AntiTerritory(ctx, g, id, query.Options{})
		case "between":
			return query.Between(ctx, g, id, end, query.Options{})
		default:
			return nil, errors.New("unknown query")
		}
	}
	sub, e := runQuery()
	if e != nil {
		return e
	}
	meta := ggpb.Query{Name: *kind, Node: &origin}
	if *kind == "between" {
		meta.Node = nil
		meta.From = &origin
		meta.To = &target
	}
	encode := func(out io.Writer) error {
		if *format == "json" {
			return graphjson.Write(ctx, out, g, sub, graphjson.Query{Name: meta.Name, Node: meta.Node, From: meta.From, To: meta.To})
		}
		return ggpb.WriteOptions(ctx, out, g, sub, meta, ggpb.Options{InlineSymbols: *format == "ggpb-inline"})
	}
	for i := 0; i < *warm; i++ {
		if e = encode(io.Discard); e != nil {
			return e
		}
	}
	runtime.GC()
	if *cp != "" {
		f, err := os.Create(*cp)
		if err != nil {
			return err
		}
		defer f.Close()
		if e = pprof.StartCPUProfile(f); e != nil {
			return e
		}
		defer pprof.StopCPUProfile()
	}
	var before, after runtime.MemStats
	for i := 0; i < *repeats; i++ {
		queryMS, queryCPU := float64(0), float64(0)
		if *queryEach {
			c := cpu()
			t := time.Now()
			sub, e = runQuery()
			if e != nil {
				return e
			}
			queryMS = float64(time.Since(t)) / float64(time.Millisecond)
			queryCPU = cpu() - c
		}
		var out io.Writer = io.Discard
		var f *os.File
		if *output != "" {
			f, e = os.Create(*output)
			if e != nil {
				return e
			}
			out = f
		}
		runtime.ReadMemStats(&before)
		c := cpu()
		t := time.Now()
		e = encode(out)
		if f != nil {
			closeErr := f.Close()
			if e == nil {
				e = closeErr
			}
		}
		if e != nil {
			return e
		}
		elapsed := float64(time.Since(t)) / float64(time.Millisecond)
		used := cpu() - c
		runtime.ReadMemStats(&after)
		bytes := int64(0)
		if f != nil {
			st, err := os.Stat(*output)
			if err != nil {
				return err
			}
			bytes = st.Size()
		}
		r := map[string]any{"Format": *format, "Repeat": i, "Nodes": sub.NodeCount(), "Edges": sub.EdgeCount(), "EncodeMS": elapsed, "QueryMS": queryMS, "QueryCPUTimeMS": queryCPU, "TotalMS": elapsed + queryMS, "TotalCPUTimeMS": used + queryCPU, "CPUTimeMS": used, "AllocBytes": after.TotalAlloc - before.TotalAlloc, "Allocs": after.Mallocs - before.Mallocs, "HeapAfter": after.HeapAlloc, "Bytes": bytes}
		if e = json.NewEncoder(os.Stdout).Encode(r); e != nil {
			return e
		}
	}
	if *mp != "" {
		runtime.GC()
		f, err := os.Create(*mp)
		if err != nil {
			return err
		}
		e = pprof.WriteHeapProfile(f)
		return errors.Join(e, f.Close())
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
