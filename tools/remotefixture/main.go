package main

import (
	"context"
	"flag"
	"fmt"
	"gophergraph/internal/benchfixture"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	n := flag.Int("nodes", 4000, "nodes and cycle edges")
	scalar := flag.Int("scalar", 32768, "property bytes, interned once in snapshot")
	output := flag.String("output", "", "snapshot output")
	flag.Parse()
	if *output == "" {
		return fmt.Errorf("output required")
	}
	ctx := context.Background()
	g, err := benchfixture.StreamingGraph(ctx, *n, *scalar)
	if err != nil {
		return err
	}
	defer g.Close()
	_, err = snapshot.Write(ctx, g, file.New(*output))
	return err
}
