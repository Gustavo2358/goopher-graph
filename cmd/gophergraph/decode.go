package main

import (
	"context"
	"errors"
	"flag"
	"gophergraph/ggpb"
	"gophergraph/ingest/adapters/filesystem"
	"io"
	"os"
)

func decodeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) (code int) {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "GGPB file (required)")
	output := fs.String("output", "", "JSON output file (default stdout)")
	format := fs.String("format", "json", "json")
	if code := parse(fs, args); code >= 0 {
		return code
	}
	if *input == "" || *format != "json" || (hasFlag(fs, "output") && *output == "") {
		return failure(stderr, errors.New("missing input or invalid output format"), 2)
	}
	if e := filesystem.ValidateOutput(*input, *output); e != nil {
		return failure(stderr, e, 2)
	}
	in, e := os.Open(*input)
	if e != nil {
		return failure(stderr, e, 1)
	}
	defer func() {
		if e := in.Close(); e != nil {
			code = failure(stderr, e, 1)
		}
	}()
	out := stdout
	var file *os.File
	if *output != "" {
		file, e = os.Create(*output)
		if e != nil {
			return failure(stderr, e, 1)
		}
		out = file
	}
	e = ggpb.JSON(ctx, out, in)
	if file != nil {
		e = errors.Join(e, file.Close())
	}
	if e != nil {
		return failure(stderr, e, 1)
	}
	return 0
}
