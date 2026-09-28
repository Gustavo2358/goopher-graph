package ports // gophergraph/snapshot/ports

import (
	"context"
	"io"
)

type Backing interface {
	Bytes() []byte // acesso privilegiado pelo codec; estável até Close
	Close() error
}

type Source interface {
	Acquire(ctx context.Context) (Backing, error)
}

type Publication uint8

const (
	NotPublished Publication = iota
	PublishedDurable
	PublishedUncertain
)

type Sink interface {
	Begin(ctx context.Context, totalSize uint64) (Transaction, error)
}

type Transaction interface {
	io.Writer
	Seal(ctx context.Context) (Backing, error)
	Commit(ctx context.Context) (Publication, error)
	Abort() error // idempotente; não remove o destino após publicação
}
