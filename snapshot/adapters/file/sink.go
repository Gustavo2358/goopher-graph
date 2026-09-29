// Package file publishes snapshots through same-directory staging and rename.
package file

import (
	"context"
	"errors"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/snapshot/ports"
	"io"
	"os"
	"path/filepath"
)

type operations struct {
	sync, close, dirSync func(*os.File) error
	rename               func(string, string) error
}
type Sink struct {
	path string
	ops  operations
}

func New(path string) *Sink {
	return &Sink{path: path, ops: operations{sync: (*os.File).Sync, close: (*os.File).Close, dirSync: (*os.File).Sync, rename: os.Rename}}
}

type transaction struct {
	file                       *os.File
	temp, destination          string
	size, written              uint64
	sealed, published, aborted bool
	leases                     int
	ops                        operations
}

func (s *Sink) Begin(ctx context.Context, size uint64) (ports.Transaction, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	path, e := filepath.Abs(s.path)
	if e != nil {
		return nil, e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".gophergraph-*")
	if e != nil {
		return nil, e
	}
	return &transaction{file: f, temp: f.Name(), destination: path, size: size, ops: s.ops}, nil
}
func (t *transaction) Write(p []byte) (int, error) {
	if t.sealed || t.aborted || t.file == nil {
		return 0, errors.New("transaction is not writable")
	}
	if uint64(len(p)) > t.size-t.written {
		return 0, io.ErrShortWrite
	}
	n, e := t.file.Write(p)
	t.written += uint64(n)
	return n, e
}

type lease struct {
	ports.Backing
	owner  *transaction
	closed bool
	err    error
}

func (l *lease) Close() error {
	if l.closed {
		return l.err
	}
	l.closed = true
	l.err = l.Backing.Close()
	l.owner.leases--
	return l.err
}
func (t *transaction) Seal(ctx context.Context) (ports.Backing, error) {
	if t.sealed || t.aborted || t.file == nil || t.written != t.size {
		return nil, errors.New("incomplete or closed transaction")
	}
	b, e := mmap.New(t.temp).Acquire(ctx)
	if e != nil {
		return nil, e
	}
	t.sealed = true
	t.leases++
	return &lease{Backing: b, owner: t}, nil
}
func (t *transaction) Commit(ctx context.Context) (ports.Publication, error) {
	if t.published {
		return ports.PublishedDurable, errors.New("transaction already committed")
	}
	if !t.sealed || t.aborted || t.file == nil || t.leases != 0 {
		return ports.NotPublished, errors.New("transaction not sealed or validation lease active")
	}
	if e := ctx.Err(); e != nil {
		return ports.NotPublished, e
	}
	if e := t.ops.sync(t.file); e != nil {
		return ports.NotPublished, e
	}
	e := t.ops.close(t.file)
	t.file = nil
	if e != nil {
		return ports.NotPublished, e
	}
	dir, e := os.Open(filepath.Dir(t.destination))
	if e != nil {
		return ports.NotPublished, e
	}
	if e := ctx.Err(); e != nil {
		return ports.NotPublished, errors.Join(e, dir.Close())
	}
	if e = t.ops.rename(t.temp, t.destination); e != nil {
		return ports.NotPublished, errors.Join(e, dir.Close())
	}
	t.published = true
	// After rename cancellation cannot restore the old name. Finish the durability
	// operation and report the actual visibility of the new snapshot.
	syncErr := t.ops.dirSync(dir)
	closeErr := dir.Close()
	if syncErr != nil || closeErr != nil {
		return ports.PublishedUncertain, errors.Join(syncErr, closeErr)
	}
	return ports.PublishedDurable, nil
}
func (t *transaction) Abort() error {
	if t.published || t.aborted {
		return nil
	}
	if t.leases != 0 {
		return errors.New("validation lease still active")
	}
	t.aborted = true
	var e error
	if t.file != nil {
		e = t.file.Close()
		t.file = nil
	}
	removeErr := os.Remove(t.temp)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(e, removeErr)
}
