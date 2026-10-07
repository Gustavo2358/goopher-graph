//go:build linux && amd64

// Package mmap owns read-only Linux mappings. Published inodes must remain unchanged.
package mmap

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"gophergraph/snapshot/ports"
	"math"
	"os"
)

type Source struct{ path string }

func New(path string) *Source { return &Source{path} }

type backing struct {
	data   []byte
	closed bool
	err    error
	locked bool
}

func (b *backing) Bytes() []byte { return b.data }
func (b *backing) Close() error {
	if b.closed {
		return b.err
	}
	b.closed = true
	if b.locked {
		b.err = unix.Munlock(b.data)
	}
	b.err = errors.Join(b.err, unix.Munmap(b.data))
	b.data = nil
	return b.err
}
func (s *Source) Acquire(ctx context.Context) (ports.Backing, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(s.path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil {
		return nil, errors.Join(e, f.Close())
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > math.MaxInt {
		return nil, errors.Join(errors.New("mmap requires a nonempty regular file fitting the platform"), f.Close())
	}
	data, e := unix.Mmap(int(f.Fd()), 0, int(info.Size()), unix.PROT_READ, unix.MAP_PRIVATE)
	closeErr := f.Close()
	if e != nil {
		return nil, errors.Join(e, closeErr)
	}
	if closeErr != nil {
		return nil, errors.Join(closeErr, unix.Munmap(data))
	}
	if e := ctx.Err(); e != nil {
		return nil, errors.Join(e, unix.Munmap(data))
	}
	return &backing{data: data}, nil
}
