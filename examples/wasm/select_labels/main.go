// select_labels selects (any requested label) AND a string property equality.
// Arguments: property, value, then zero or more labels. No labels selects none.
package main

import (
	"os"

	"gophergraph/wasmquery/sdk"
)

func main() {
	if len(os.Args) < 3 {
		os.Exit(2)
	}
	q := sdk.New()
	selected := q.NodesWithAnyLabelAndProperty(os.Args[3:], os.Args[1], sdk.String(os.Args[2]))
	sub := selected.Induced()
	selected.Release()
	if err := q.Return(sub); err != nil {
		os.Exit(1)
	}
}
