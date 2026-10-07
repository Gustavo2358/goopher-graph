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
	neighbors := q.Nodes(os.Args[1]).Out()
	selected := q.NodesWithAnyLabelAndProperty([]string{os.Args[2]}, os.Args[3], sdk.String(os.Args[4]))
	targets := neighbors.Intersection(selected)
	neighbors.Release()
	selected.Release()
	callers := targets.In()
	edges := callers.OutE().Intersection(targets.InE())
	if err := q.Return(q.Subgraph(targets, edges)); err != nil {
		os.Exit(1)
	}
}
