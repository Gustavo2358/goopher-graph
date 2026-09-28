//go:build linux

// Package filesystem exposes immediate regular files in an explicit catalog.
package filesystem

import (
	"context"
	"errors"
	"gophergraph/ingest/ports"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type Catalog struct{ root string }

func New(path string) (*Catalog, error) {
	root, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	info, e := os.Stat(root)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() {
		return nil, errors.New("catalog is not a directory")
	}
	return &Catalog{root}, nil
}
func (c *Catalog) List(ctx context.Context) ([]ports.Entry, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(c.root)
	if e != nil {
		return nil, e
	}
	out := make([]ports.Entry, 0, len(entries))
	for _, entry := range entries {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		out = append(out, ports.Entry{Key: entry.Name(), Regular: entry.Type().IsRegular()})
	}
	return out, nil
}
func (c *Catalog) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if key == "." || key == ".." || key == "" || filepath.Base(key) != key {
		return nil, errors.New("invalid catalog key")
	}
	path := filepath.Join(c.root, key)
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source is not regular")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	actual, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !actual.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("source changed file type")
	}
	return f, nil
}
