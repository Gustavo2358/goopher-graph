package ingest

import (
	"gophergraph/ingest/ports"
	"time"
)

type Completeness uint8

const (
	Complete Completeness = iota
	Partial
)

type Options struct {
	// Scratch enables external ingestion. Without it Build retains its in-memory API.
	Scratch ports.Scratch
	// MemoryBudget bounds sort buffers, not the process RSS or decoder record.
	// Zero selects 64 MiB. Minimum: 1 MiB.
	MemoryBudget    uint64
	IndexProperties []string
	Limits          ports.Limits
}

type CatalogReport struct {
	SourcesSeen           uint64
	SourcesCompleted      uint64
	SourcesRejectedHeader uint64
	SourcesIOFailed       uint64
	SourcesInterrupted    uint64
	SourcesNonRegular     uint64
	RecordsSeen           uint64
	RecordsRejected       uint64
	RecordsStaged         uint64
}

type PhaseTimes struct {
	IngestMerge  time.Duration
	Canonicalize time.Duration
}

type Report struct {
	Times                             PhaseTimes
	Completeness                      Completeness
	NodeSources                       CatalogReport
	EdgeSources                       CatalogReport
	Nodes                             uint64
	Edges                             uint64
	PropertyConflictGroups            uint64
	PropertyCardinalityConflictGroups uint64
	QuarantinedEdgeIDs                uint64
	Warnings                          uint64
}
