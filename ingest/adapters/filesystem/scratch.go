package filesystem

import (
	"context"
	"errors"
	"gophergraph/ingest/ports"
	"gophergraph/snapshot/adapters/mmap"
	"os"
)

// Scratch stores temporary build files under Dir (empty uses os.TempDir).
// Use a disk filesystem for inputs larger than RAM; tmpfs consumes memory.
type Scratch struct{ Dir string }

func (s Scratch) New(ctx context.Context) (ports.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(s.Dir, ".gophergraph-build-")
	if err != nil {
		return nil, err
	}
	return &workspace{dir: dir, files: make(map[ports.ScratchFile]*os.File)}, nil
}

type workspace struct {
	dir    string
	files  map[ports.ScratchFile]*os.File
	closed bool
}

func (w *workspace) Create() (ports.ScratchFile, error) {
	if w.closed {
		return nil, errors.New("closed build workspace")
	}
	f, err := os.CreateTemp(w.dir, "part-")
	if err == nil {
		w.files[f] = f
	}
	return f, err
}
func (w *workspace) Remove(file ports.ScratchFile) error {
	f, ok := w.files[file]
	if !ok {
		return errors.New("unknown scratch file")
	}
	delete(w.files, file)
	return errors.Join(f.Close(), os.Remove(f.Name()))
}
func (w *workspace) Map(ctx context.Context, file ports.ScratchFile) (ports.Mapping, error) {
	f, ok := w.files[file]
	if !ok {
		return nil, errors.New("unknown scratch file")
	}
	return mmap.New(f.Name()).Acquire(ctx)
}
func (w *workspace) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	var err error
	for f := range w.files {
		err = errors.Join(err, w.Remove(f))
	}
	return errors.Join(err, os.RemoveAll(w.dir))
}
