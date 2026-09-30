// between composes territory(A) intersection anti-territory(B).
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
	nodes := q.Nodes(os.Args[1]).Territory().Intersection(q.Nodes(os.Args[2]).AntiTerritory())
	if err := q.Return(nodes.Induced()); err != nil {
		os.Exit(1)
	}
}
