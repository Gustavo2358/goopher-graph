// Command shared_targets demonstrates a query composed only from public APIs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	shared "gophergraph/examples/shared_targets"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"os"
)

func run(ctx context.Context, args []string, out io.Writer) (err error) {
	if len(args) != 3 {
		return errors.New("usage: shared_targets SNAPSHOT A B")
	}
	g, err := snapshot.Open(ctx, mmap.New(args[0]))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, g.Close()) }()
	set, err := shared.Execute(ctx, g, args[1], args[2])
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	it := set.Iterator()
	for it.Next() {
		id, e := g.NodeExternalID(it.ID())
		if e != nil {
			return e
		}
		if e = encoder.Encode(id); e != nil {
			return e
		}
	}
	return nil
}
func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
