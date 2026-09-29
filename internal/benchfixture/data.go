// Package benchfixture generates deterministic synthetic CSV datasets for measurements.
package benchfixture

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write produces n nodes and 5*n edges in two disconnected components, with
// chains, overlapping diamonds, cycles, two hubs, loops and typed properties.
func Write(root string, n int) error {
	if n < 10 || n > 1000000 {
		return errors.New("benchmark size must be 10..1000000")
	}
	for _, dir := range []string{"nodes", "edges"} {
		if e := os.MkdirAll(filepath.Join(root, dir), 0700); e != nil {
			return e
		}
	}
	write := func(role, header string, body func(*bufio.Writer) error) error {
		f, e := os.Create(filepath.Join(root, role, "data.csv"))
		if e != nil {
			return e
		}
		w := bufio.NewWriterSize(f, 65536)
		_, e = fmt.Fprintln(w, header)
		if e == nil {
			e = body(w)
		}
		return errors.Join(e, w.Flush(), f.Close())
	}
	if e := write("nodes", "~id,~label,group:String,rank:Int", func(w *bufio.Writer) error {
		for i := 0; i < n; i++ {
			if _, e := fmt.Fprintf(w, "n%09d,TYPE;GROUP,g%d,%d\n", i, i%10, i); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		return e
	}
	return write("edges", "~id,~from,~to,~label,score:Int", func(w *bufio.Writer) error {
		for i := 0; i < n; i++ {
			base, end := 0, n*9/10
			if i >= end {
				base, end = end, n
			}
			length := end - base
			src := [5]int{i, i, i, base, i}
			dst := [5]int{base + (i-base+1)%length, base + (i-base+2)%length, base, i, i}
			labels := [5]string{"L", "L", "L", "M", "U"}
			for k := 0; k < 5; k++ {
				if _, e := fmt.Fprintf(w, "e%09d,n%09d,n%09d,%s,%d\n", 5*i+k, src[k], dst[k], labels[k], i%100); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
