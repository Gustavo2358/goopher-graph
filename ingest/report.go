package ingest

import (
	"gophergraph/ingest/ports"
)

type Completeness uint8

const (
	Complete Completeness = iota
	Partial
)

type Options struct {
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

type Report struct {
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
