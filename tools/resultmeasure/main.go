// Command resultmeasure measures separate producer and consumer processes.
// Compile before measuring; fixture generation/build are separate invocations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type report struct {
	Mode, Format, Query                                     string
	Nodes, Edges, Bytes                                     uint64
	OpenMS, QueryMS, EncodeMS, DecodeMS, TotalMS, CPUTimeMS float64
	PeakRSSKiB                                              int64
	AllocBytes, Allocs, HeapBefore, HeapAfter               uint64
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
func cpu() float64 {
	var r syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &r)
	return float64(r.Utime.Sec+r.Stime.Sec)*1000 + float64(r.Utime.Usec+r.Stime.Usec)/1000
}
func run() error {
	mode := flag.String("mode", "encode", "encode or decode")
	format := flag.String("format", "ggpb", "json, ggpb or ggpb-inline")
	path := flag.String("snapshot", "", "snapshot")
	input := flag.String("input", "", "consumer file")
	output := flag.String("output", "", "producer file")
	kind := flag.String("query", "territory", "territory, anti-territory or between")
	node := flag.String("node", "n000000000", "origin")
	to := flag.String("to", "n000000010", "between target")
	flag.Parse()
	ctx := context.Background()
	r := report{Mode: *mode, Format: *format, Query: *kind}
	if *mode == "encode" {
		t := time.Now()
		g, e := snapshot.Open(ctx, mmap.New(*path))
		if e != nil {
			return e
		}
		r.OpenMS = ms(time.Since(t))
		defer g.Close()
		id, e := g.FindNode(*node)
		if e != nil {
			return e
		}
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		r.HeapBefore = before.HeapAlloc
		t = time.Now()
		c := cpu()
		var sub *query.Subgraph
		meta := ggpb.Query{Name: *kind, Node: node}
		switch *kind {
		case "territory":
			sub, e = query.Territory(ctx, g, id, query.Options{})
		case "anti-territory":
			sub, e = query.AntiTerritory(ctx, g, id, query.Options{})
		case "between":
			end, err := g.FindNode(*to)
			if err != nil {
				return err
			}
			sub, e = query.Between(ctx, g, id, end, query.Options{})
			meta.Node = nil
			meta.From = node
			meta.To = to
		default:
			return errors.New("unknown query")
		}
		if e != nil {
			return e
		}
		r.QueryMS = ms(time.Since(t))
		r.Nodes = sub.NodeCount()
		r.Edges = sub.EdgeCount()
		out, e := os.Create(*output)
		if e != nil {
			return e
		}
		t = time.Now()
		switch *format {
		case "json":
			e = graphjson.Write(ctx, out, g, sub, graphjson.Query{Name: meta.Name, Node: meta.Node, From: meta.From, To: meta.To})
		case "ggpb", "ggpb-inline":
			e = ggpb.WriteOptions(ctx, out, g, sub, meta, ggpb.Options{InlineSymbols: *format == "ggpb-inline"})
		default:
			e = errors.New("unknown format")
		}
		e = errors.Join(e, out.Close())
		if e != nil {
			return e
		}
		r.EncodeMS = ms(time.Since(t))
		r.TotalMS = r.QueryMS + r.EncodeMS
		r.CPUTimeMS = cpu() - c
		runtime.ReadMemStats(&after)
		r.AllocBytes = after.TotalAlloc - before.TotalAlloc
		r.Allocs = after.Mallocs - before.Mallocs
		r.HeapAfter = after.HeapAlloc
		info, e := os.Stat(*output)
		if e != nil {
			return e
		}
		r.Bytes = uint64(info.Size())
	} else if *mode == "decode" {
		in, e := os.Open(*input)
		if e != nil {
			return e
		}
		defer in.Close()
		info, e := in.Stat()
		if e != nil {
			return e
		}
		r.Bytes = uint64(info.Size())
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		r.HeapBefore = before.HeapAlloc
		t := time.Now()
		c := cpu()
		if *format == "json" {
			r.Nodes, r.Edges, e = consumeJSON(in)
		} else {
			rd := ggpb.NewReader(in)
			for {
				b, err := rd.Next(ctx)
				if err == io.EOF {
					break
				}
				if err != nil {
					e = err
					break
				}
				if end := b.GetEnd(); end != nil {
					r.Nodes = end.Nodes
					r.Edges = end.Edges
				}
			}
		}
		if e != nil {
			return e
		}
		r.DecodeMS = ms(time.Since(t))
		r.TotalMS = r.DecodeMS
		r.CPUTimeMS = cpu() - c
		runtime.ReadMemStats(&after)
		r.AllocBytes = after.TotalAlloc - before.TotalAlloc
		r.Allocs = after.Mallocs - before.Mallocs
		r.HeapAfter = after.HeapAlloc
	} else {
		return errors.New("unknown mode")
	}
	var usage syscall.Rusage
	if e := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); e != nil {
		return e
	}
	r.PeakRSSKiB = usage.Maxrss
	// /proc reports this executable's HWM; getrusage can inherit fork-parent RSS.
	if data, e := os.ReadFile("/proc/self/status"); e == nil {
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) > 1 && f[0] == "VmHWM:" {
				r.PeakRSSKiB, _ = strconv.ParseInt(f[1], 10, 64)
			}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}

// JSON consumer keeps a single entity and parses every property scalar according
// to its declared graph type. GGPB Reader likewise validates every typed scalar.
type entity struct {
	ID, Source, Target, Label string
	Labels                    []string
	Properties                []struct {
		Key, Type string
		Value     json.RawMessage
	}
}

func consumeJSON(in io.Reader) (nodes, edges uint64, err error) {
	d := json.NewDecoder(in)
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return 0, 0, errors.New("invalid JSON envelope")
	}
	var counts struct{ Nodes, Edges uint64 }
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return 0, 0, e
		}
		switch k {
		case "nodes", "edges":
			tok, e = d.Token()
			if e != nil || tok != json.Delim('[') {
				return 0, 0, errors.New("invalid JSON entities")
			}
			for d.More() {
				var v entity
				if e = d.Decode(&v); e != nil {
					return 0, 0, e
				}
				for _, p := range v.Properties {
					if e = scalar(p.Type, p.Value); e != nil {
						return 0, 0, e
					}
				}
				if k == "nodes" {
					nodes++
				} else {
					edges++
				}
			}
			if _, e = d.Token(); e != nil {
				return 0, 0, e
			}
		case "counts":
			if e = d.Decode(&counts); e != nil {
				return 0, 0, e
			}
		default:
			var raw json.RawMessage
			if e = d.Decode(&raw); e != nil {
				return 0, 0, e
			}
		}
	}
	if _, e = d.Token(); e != nil {
		return 0, 0, e
	}
	if nodes != counts.Nodes || edges != counts.Edges {
		return 0, 0, errors.New("JSON counts")
	}
	if _, e = d.Token(); e != io.EOF {
		return 0, 0, errors.New("JSON trailing bytes")
	}
	return nodes, edges, nil
}
func scalar(kind string, raw json.RawMessage) error {
	switch kind {
	case "Bool":
		var b bool
		return json.Unmarshal(raw, &b)
	case "Byte", "Short", "Int", "Long":
		s := string(raw)
		k := graph.IntKind
		switch kind {
		case "Byte":
			k = graph.ByteKind
		case "Short":
			k = graph.ShortKind
		case "Long":
			k = graph.LongKind
			if e := json.Unmarshal(raw, &s); e != nil {
				return e
			}
		}
		n, e := strconv.ParseInt(s, 10, 64)
		if e != nil {
			return e
		}
		_, e = graph.IntegerValue(k, n)
		return e
	case "Float", "Double":
		s := string(raw)
		if len(s) > 0 && s[0] == '"' {
			if e := json.Unmarshal(raw, &s); e != nil {
				return e
			}
		}
		bits := 64
		if kind == "Float" {
			bits = 32
		}
		_, e := strconv.ParseFloat(s, bits)
		return e
	case "String", "Date", "Datetime":
		var s string
		return json.Unmarshal(raw, &s)
	}
	return errors.New("unknown JSON property type")
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
