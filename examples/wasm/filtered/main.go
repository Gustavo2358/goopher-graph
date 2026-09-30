// filtered selects typed, labelled targets, then callers and connecting edges.
package main

import (
	"gophergraph/wasmquery/sdk"
	"os"
)

func main() {
	if len(os.Args) != 5 {
		os.Exit(2)
	}
	q := sdk.New()
	targets := q.Nodes(os.Args[1]).Out().HasLabel(os.Args[2]).Has(os.Args[3], sdk.String(os.Args[4]))
	callers := targets.In()
	edges := callers.OutE().Intersection(targets.InE())
	if err := q.Return(q.Subgraph(targets, edges)); err != nil {
		os.Exit(1)
	}
}
