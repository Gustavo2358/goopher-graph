// Command gophergraph builds and queries immutable local graph snapshots.
package main

import (
	"fmt"
	"io"
	"os"
)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, err := fmt.Fprintln(stdout, "Usage: gophergraph --help\nGopherGraph: immutable directed property multigraphs."); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintln(stderr, "usage: gophergraph --help")
	return 2
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
