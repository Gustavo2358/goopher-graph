// shared_targets returns the common reachable nodes and their induced edges.
package main

import (
	"gophergraph/wasmquery/sdk"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	q := sdk.New()
	shared := q.Nodes(os.Args[1]).Territory().Intersection(q.Nodes(os.Args[2]).Territory())
	if err := q.Return(shared.Induced()); err != nil {
		os.Exit(1)
	}
}
