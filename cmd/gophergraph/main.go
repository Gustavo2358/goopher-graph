// Command gophergraph builds and queries immutable local graph snapshots.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const help = "Usage: gophergraph <command> [options]\nCommands: build, territory, anti-territory, between, wasm, decode\nUse gophergraph <command> --help for options."

func run(args []string, stdout, stderr io.Writer) int {
	return runContext(context.Background(), args, stdout, stderr)
}
func runContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, e := fmt.Fprintln(stdout, help); e != nil {
			return 1
		}
		return 0
	}
	if len(args) == 0 {
		return failure(stderr, errors.New(help), 2)
	}
	switch args[0] {
	case "decode":
		return decodeCommand(ctx, args[1:], stdout, stderr)
	case "wasm":
		return wasmCommand(ctx, args[1:], stdout, stderr)
	case "build":
		return buildCommand(ctx, args[1:], stdout, stderr)
	case "territory", "anti-territory", "between":
		return queryCommand(ctx, args[0], args[1:], stdout, stderr)
	default:
		return failure(stderr, errors.New(help), 2)
	}
}
func failure(stderr io.Writer, e error, code int) int {
	if _, err := fmt.Fprintln(stderr, e); err != nil {
		return 1
	}
	return code
}
func parse(fs *flag.FlagSet, args []string) int {
	e := fs.Parse(args)
	if errors.Is(e, flag.ErrHelp) {
		return 0
	}
	if e != nil || fs.NArg() != 0 {
		return 2
	}
	return -1
}

type repeated []string

func (r *repeated) String() string     { return fmt.Sprint([]string(*r)) }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }
func hasFlag(fs *flag.FlagSet, key string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == key {
			found = true
		}
	})
	return found
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := runContext(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
