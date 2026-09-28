package ports // gophergraph/ingest/ports

import (
	"context"
	"gophergraph/graph"
	"io"
)

type Role uint8

const (
	Nodes Role = iota
	Edges
)

type Entry struct {
	Key     string
	Regular bool
}

type Catalog interface {
	List(ctx context.Context) ([]Entry, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

type Limits struct {
	MaxRecordBytes uint64
	MaxColumns     uint32
}

type Location struct {
	Role   Role
	Source string
	Record uint64
	Line   uint64
	Column string
}

type Severity uint8

const (
	Info Severity = iota
	Warning
	Rejection
)

type Diagnostic struct {
	Severity      Severity
	Code          string // código estável, não classificação por texto
	Location      Location
	EntityID      string
	EntityIDKnown bool // ID vazio também pode ser conhecido
	Message       string
}

type DiagnosticSink interface {
	Emit(ctx context.Context, event Diagnostic) error
}

type Cardinality uint8

const (
	Set Cardinality = iota
	Single
)

type Property struct {
	Key         string
	Value       graph.Value
	Cardinality Cardinality
}

type Record struct {
	Role       Role
	ID         string
	Source     string // usado somente em edge
	Target     string // usado somente em edge
	Labels     []string
	Properties []Property
	Location   Location
}

type Event struct {
	Record    *Record // exatamente um de Record/Rejection
	Rejection *Diagnostic
	Warnings  []Diagnostic // somente record válido
}

type RecordReader interface {
	Next(ctx context.Context) (Event, error) // EOF normal; outros erros são classificados
}

type Decoder interface {
	New(ctx context.Context, role Role, entry Entry, input io.Reader, limits Limits) (RecordReader, error)
}

type FailureKind uint8

const (
	InvalidHeader FailureKind = iota
	SourceIO
	UnrecoverableCSV
	ResourceLimit
)

type SourceError struct {
	Kind     FailureKind
	Location Location
	Cause    error
}

func (e *SourceError) Error() string { return "source failure: " + e.Cause.Error() }
func (e *SourceError) Unwrap() error { return e.Cause }
