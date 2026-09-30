package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	diagnostic "gophergraph/ingest/adapters/stderr"
	ingestports "gophergraph/ingest/ports"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/ports"
	"io"
	"math"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func buildCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nodes := fs.String("nodes", "", "node catalog directory (required)")
	edges := fs.String("edges", "", "edge catalog directory (required)")
	output := fs.String("output", "", "snapshot destination (required)")
	budget := fs.Uint64("memory-budget", 64<<20, "sort buffer budget in bytes (minimum 1 MiB; excludes record buffers and mappings)")
	tempDir := fs.String("temp-dir", "", "build scratch parent directory (default: output directory)")
	limit := fs.Uint64("max-record-bytes", 67108864, "maximum logical record bytes (minimum 1024)")
	var keys repeated
	fs.Var(&keys, "index-property", "property key to index (repeatable)")
	if code := parse(fs, args); code >= 0 {
		return code
	}
	if *budget < 1<<20 || *budget > uint64(math.MaxInt) {
		return failure(stderr, errors.New("memory-budget must be 1 MiB..MaxInt"), 2)
	}
	if *nodes == "" || *edges == "" || *output == "" || *limit < 1024 || *limit > uint64(math.MaxInt) {
		return failure(stderr, errors.New("build requires --nodes, --edges, --output and max-record-bytes >= 1024"), 2)
	}
	for _, key := range keys {
		if key == "" || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
			return failure(stderr, errors.New("invalid index property key"), 2)
		}
	}
	if e := filesystem.ValidateBuildPaths(*nodes, *edges, *output); e != nil {
		return failure(stderr, e, 2)
	}
	n, e := filesystem.New(*nodes)
	if e != nil {
		return failure(stderr, e, 2)
	}
	r, e := filesystem.New(*edges)
	if e != nil {
		return failure(stderr, e, 2)
	}
	if *tempDir == "" {
		*tempDir = filepath.Dir(*output)
	}
	if e := filesystem.ValidateBuildPaths(*nodes, *edges, *tempDir); e != nil {
		return failure(stderr, fmt.Errorf("temp-dir: %w", e), 2)
	}
	g, report, e := ingest.Build(ctx, n, r, neptune.Decoder{}, diagnostic.New(stderr), ingest.Options{Scratch: filesystem.Scratch{Dir: *tempDir}, MemoryBudget: *budget, IndexProperties: keys, Limits: ingestports.Limits{MaxRecordBytes: *limit}})
	if e != nil {
		return failure(stderr, e, 1)
	}
	writeStart := time.Now()
	publication, e := snapshot.Write(ctx, g, file.New(*output))
	writeDuration := time.Since(writeStart)
	closeErr := g.Close()
	if e != nil || closeErr != nil {
		state := "NOT_PUBLISHED"
		if publication == ports.PublishedUncertain {
			state = "PUBLISHED_UNCERTAIN: new snapshot is visible; durability not confirmed"
		} else if publication == ports.PublishedDurable {
			state = "PUBLISHED_DURABLE"
		}
		return failure(stderr, fmt.Errorf("%s: %w", state, errors.Join(e, closeErr)), 1)
	}
	completeness := "COMPLETE"
	if report.Completeness == ingest.Partial {
		completeness = "PARTIAL"
	}
	_, e = fmt.Fprintf(stdout, "PUBLISHED_DURABLE %s nodes=%d edges=%d\nnode_sources=%+v\nedge_sources=%+v\nproperty_conflicts=%d cardinality_conflicts=%d quarantined_edges=%d warnings=%d\n", completeness, report.Nodes, report.Edges, report.NodeSources, report.EdgeSources, report.PropertyConflictGroups, report.PropertyCardinalityConflictGroups, report.QuarantinedEdgeIDs, report.Warnings)
	if e == nil {
		_, e = fmt.Fprintf(stdout, "ingest_merge=%s canonicalize_csr_indexes=%s write_validate_commit=%s\n", report.Times.IngestMerge, report.Times.Canonicalize, writeDuration)
	}
	if e != nil {
		return failure(stderr, fmt.Errorf("PUBLISHED_DURABLE: report output failed: %w", e), 1)
	}
	return 0
}
