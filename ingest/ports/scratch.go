package ports

import (
	"context"
	"io"
)

// Scratch creates a private workspace for one bounded build. The builder owns
// the workspace and closes it on every exit. Mappings outlive workspace removal.
type Scratch interface {
	New(context.Context) (Workspace, error)
}
type ScratchFile interface {
	io.Writer
	io.ReaderAt
	io.Closer
}
type Mapping interface {
	Bytes() []byte
	Close() error
}
type Workspace interface {
	Create() (ScratchFile, error)
	Remove(ScratchFile) error
	Map(context.Context, ScratchFile) (Mapping, error)
	Close() error
}
