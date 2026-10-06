// Command encodingmeasure profiles encoding on an opened snapshot/result.
// Query and opening costs are excluded. Build before measuring.
package main

import (
	"context"
	"encoding/json"
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
	sub, e := query.Territory(ctx, g, id, query.Options{})
	if e != nil {
		return e
	}
	origin := "n000000000"
	meta := ggpb.Query{Name: "territory", Node: &origin}
	if *empty {
		end, err := g.FindNode("n000000090")
		if err != nil {
			return err
		}
		sub, e = query.Between(ctx, g, id, end, query.Options{})
		if e != nil {
			return e
		}
		meta = ggpb.Query{Name: "between", From: &origin, To: new(string)}
		*meta.To = "n000000090"
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
		r := map[string]any{"Format": *format, "Repeat": i, "Nodes": sub.NodeCount(), "Edges": sub.EdgeCount(), "EncodeMS": elapsed, "CPUTimeMS": used, "AllocBytes": after.TotalAlloc - before.TotalAlloc, "Allocs": after.Mallocs - before.Mallocs, "HeapAfter": after.HeapAlloc, "Bytes": bytes}
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
		return errorsJoin(e, f.Close())
	}
	return nil
}
func errorsJoin(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
