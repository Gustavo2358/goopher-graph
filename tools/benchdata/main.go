// Command benchdata generates the synthetic input used by benchmarks.
package main

import (
	"flag"
	"fmt"
	"gophergraph/internal/benchfixture"
	"os"
)

func main() {
	n := flag.Int("nodes", 1000, "node count; produces five edges per node")
	out := flag.String("output", "", "dataset directory")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "--output is required")
		os.Exit(2)
	}
	if e := benchfixture.Write(*out, *n); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
