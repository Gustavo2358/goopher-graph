package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func resolved(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e == nil {
		return p, nil
	}
	if !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil {
		return "", e
	}
	parent, e = filepath.Abs(parent)
	if e != nil {
		return "", e
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

// ValidateBuildPaths rejects aliases and destinations inside either catalog.
func ValidateBuildPaths(nodes, edges, output string) error {
	n, e := New(nodes)
	if e != nil {
		return e
	}
	r, e := New(edges)
	if e != nil {
		return e
	}
	a, e := os.Stat(n.root)
	if e != nil {
		return e
	}
	b, e := os.Stat(r.root)
	if e != nil {
		return e
	}
	if os.SameFile(a, b) {
		return errors.New("node and edge catalogs must be distinct")
	}
	out, e := resolved(output)
	if e != nil {
		return e
	}
	for _, root := range []string{n.root, r.root} {
		rel, e := filepath.Rel(root, out)
		if e != nil {
			return e
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return errors.New("output must be outside input catalogs")
		}
		info, e := os.Stat(out)
		if e == nil {
			entries, e := os.ReadDir(root)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				in, e := os.Stat(filepath.Join(root, entry.Name()))
				if e == nil && os.SameFile(info, in) {
					return errors.New("output aliases an input file")
				}
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	return nil
}

// ValidateOutput protects an opened snapshot from output truncation via aliases.
func ValidateOutput(input, output string) error {
	if output == "" {
		return nil
	}
	a, e := resolved(input)
	if e != nil {
		return e
	}
	b, e := resolved(output)
	if e != nil {
		return e
	}
	if a == b {
		return errors.New("output aliases snapshot")
	}
	left, e := os.Stat(a)
	if e != nil {
		return e
	}
	right, e := os.Stat(b)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	if os.SameFile(left, right) {
		return errors.New("output aliases snapshot")
	}
	return nil
}
