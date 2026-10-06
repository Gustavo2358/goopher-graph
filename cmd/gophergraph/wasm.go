package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"gophergraph/ggpb"
	"gophergraph/graphjson"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
)

func wasmCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet("wasm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("snapshot", "", "snapshot path (required)")
	module := fs.String("module", "", "compiled .wasm path (required)")
	output := fs.String("output", "", "output file (default stdout)")
	format := fs.String("format", "json", "json or ggpb")
	timeout := fs.Duration("timeout", 5*time.Second, "maximum query execution time")
	var argv repeated
	fs.Var(&argv, "arg", "query argument, in order (repeatable)")
	if code := parse(fs, args); code >= 0 {
		return code
	}
	if *path == "" || *module == "" || *timeout <= 0 || (*format != "json" && *format != "ggpb") || (hasFlag(fs, "output") && *output == "") {
		return failure(stderr, errors.New("snapshot, module, positive timeout and json/ggpb format required"), 2)
	}
	if err := filesystem.ValidateOutput(*path, *output); err != nil {
		return failure(stderr, err, 2)
	}
	if err := filesystem.ValidateOutput(*module, *output); err != nil {
		return failure(stderr, err, 2)
	}
	// Bound reads even when a file grows after opening or is a nonregular source.
	f, err := os.Open(*module)
	if err != nil {
		return failure(stderr, err, 1)
	}
	const maxModule = 16 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxModule+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return failure(stderr, err, 1)
	}
	if len(data) > maxModule {
		return failure(stderr, wasmquery.ErrLimit, 1)
	}
	runtime, err := wasmquery.New(ctx, wasmquery.Limits{Timeout: *timeout})
	if err != nil {
		return failure(stderr, err, 1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			code = failure(stderr, err, 1)
		}
	}()
	compiled, err := runtime.Compile(ctx, data)
	if err != nil {
		return failure(stderr, err, 1)
	}
	g, err := snapshot.Open(ctx, mmap.New(*path))
	if err != nil {
		return failure(stderr, err, 1)
	}
	defer func() {
		if err := g.Close(); err != nil {
			code = failure(stderr, err, 1)
		}
	}()
	if g.Metadata().PartialLoad {
		if _, err = fmt.Fprintln(stderr, "PARTIAL: query covers the accepted snapshot"); err != nil {
			return 1
		}
	}
	result, err := runtime.Execute(ctx, compiled, g, argv)
	if err != nil {
		return queryFailure(stderr, err)
	}
	defer result.Close()
	sub, err := result.Subgraph()
	if err != nil {
		return failure(stderr, err, 1)
	}
	out := stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return failure(stderr, err, 1)
		}
		out = file
	}
	if *format == "ggpb" {
		err = ggpb.Write(ctx, out, g, sub, ggpb.Query{Name: "wasm:" + compiled.SHA256()})
	} else {
		err = graphjson.Write(ctx, out, g, sub, graphjson.Query{Name: "wasm:" + compiled.SHA256()})
	}
	if file != nil {
		err = errors.Join(err, file.Close())
	}
	if err != nil {
		return failure(stderr, err, 1)
	}
	return 0
}
