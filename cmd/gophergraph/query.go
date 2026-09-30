package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"gophergraph/dot"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"os"
)

func queryCommand(ctx context.Context, command string, args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("snapshot", "", "snapshot path (required)")
	output := fs.String("output", "", "output file (default stdout)")
	format := fs.String("format", "ids", "ids, dot or json")
	include := fs.Bool("include-origin", false, "include start in ID output")
	var node, from, to string
	if command == "between" {
		fs.StringVar(&from, "from", "", "start ID (required)")
		fs.StringVar(&to, "to", "", "end ID (required)")
	} else {
		fs.StringVar(&node, "node", "", "start ID (required)")
	}
	var labels repeated
	fs.Var(&labels, "edge-label", "allowed edge label (repeatable)")
	if code := parse(fs, args); code >= 0 {
		return code
	}
	if *path == "" || (*format != "ids" && *format != "dot" && *format != "json") || (command == "between" && (!hasFlag(fs, "from") || !hasFlag(fs, "to"))) || (command != "between" && !hasFlag(fs, "node")) {
		return failure(stderr, errors.New("missing required flags or invalid format"), 2)
	}
	if hasFlag(fs, "output") && *output == "" {
		return failure(stderr, errors.New("output path is empty"), 2)
	}
	for _, label := range labels {
		if label == "" {
			return failure(stderr, errors.New("edge label is empty"), 2)
		}
	}
	if e := filesystem.ValidateOutput(*path, *output); e != nil {
		return failure(stderr, e, 2)
	}
	g, e := snapshot.Open(ctx, mmap.New(*path))
	if e != nil {
		return failure(stderr, e, 1)
	}
	defer func() {
		if e := g.Close(); e != nil {
			code = failure(stderr, e, 1)
		}
	}()
	if g.Metadata().PartialLoad {
		if _, e := fmt.Fprintln(stderr, "PARTIAL: query covers the accepted snapshot"); e != nil {
			return 1
		}
	}
	options := query.Options{}
	if labels != nil {
		options.EdgeLabels = []graph.StringID{}
		for _, label := range labels {
			id, found, e := g.FindString(label)
			if e != nil {
				return failure(stderr, e, 1)
			}
			if found {
				options.EdgeLabels = append(options.EdgeLabels, id)
			}
		}
	}
	start := node
	if command == "between" {
		start = from
	}
	id, e := g.FindNode(start)
	if e != nil {
		return queryFailure(stderr, e)
	}
	var sub *query.Subgraph
	switch command {
	case "territory":
		sub, e = query.Territory(ctx, g, id, options)
	case "anti-territory":
		sub, e = query.AntiTerritory(ctx, g, id, options)
	case "between":
		end, err := g.FindNode(to)
		if err != nil {
			return queryFailure(stderr, err)
		}
		sub, e = query.Between(ctx, g, id, end, options)
	}
	if e != nil {
		return queryFailure(stderr, e)
	}
	out := stdout
	var file *os.File
	if *output != "" {
		file, e = os.Create(*output)
		if e != nil {
			return failure(stderr, e, 1)
		}
		out = file
	}
	if *format == "dot" {
		e = dot.Write(out, g, sub)
	} else if *format == "json" {
		metadata := graphjson.Query{Name: command, EdgeLabels: labels}
		if command == "between" {
			metadata.From, metadata.To = &from, &to
		} else {
			metadata.Node = &node
		}
		e = graphjson.Write(ctx, out, g, sub, metadata)
	} else {
		omit := graph.InvalidNodeID
		if command != "between" && !*include {
			omit = id
		}
		e = writeIDs(ctx, out, g, sub, omit)
	}
	if file != nil {
		e = errors.Join(e, file.Close())
	}
	if e != nil {
		return failure(stderr, e, 1)
	}
	return 0
}
func queryFailure(stderr io.Writer, e error) int {
	code := 1
	if errors.Is(e, graph.ErrNotFound) {
		code = 3
	}
	return failure(stderr, e, code)
}
func writeIDs(ctx context.Context, out io.Writer, g *graph.Graph, sub *query.Subgraph, omit graph.NodeID) error {
	w := csv.NewWriter(out)
	if e := w.Write([]string{"id"}); e != nil {
		return e
	}
	it := sub.Nodes()
	for it.Next() {
		if e := ctx.Err(); e != nil {
			return e
		}
		if it.ID() == omit {
			continue
		}
		id, e := g.NodeExternalID(it.ID())
		if e != nil {
			return e
		}
		if id == "" {
			w.Flush()
			if e := w.Error(); e != nil {
				return e
			}
			n, e := io.WriteString(out, "\"\"\n")
			if e != nil {
				return e
			}
			if n != 3 {
				return io.ErrShortWrite
			}
		} else if e := w.Write([]string{id}); e != nil {
			return e
		}
	}
	w.Flush()
	return w.Error()
}
