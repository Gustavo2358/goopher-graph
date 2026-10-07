// Package ingest consolidates normalized node and edge contributions.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type builder struct {
	policy       NodePropertyConflictPolicy
	sequence     uint64
	ctx          context.Context
	sink         ports.DiagnosticSink
	report       Report
	nodes, edges map[string]*entity
	disk         *diskBuilder
}

func Build(ctx context.Context, nodes, edges ports.Catalog, decoder ports.Decoder, diagnostics ports.DiagnosticSink, options Options) (*graph.Graph, Report, error) {
	started := time.Now()
	if options.Scratch == nil && options.MemoryBudget != 0 {
		return nil, Report{}, errors.New("memory budget requires a scratch port")
	}
	if options.Scratch != nil {
		return buildDisk(ctx, nodes, edges, decoder, diagnostics, options)
	}
	b := &builder{ctx: ctx, sink: diagnostics, nodes: map[string]*entity{}, edges: map[string]*entity{}}
	fail := func(e error) (*graph.Graph, Report, error) { return nil, b.report, e }
	if e := ctx.Err(); e != nil {
		return fail(e)
	}
	if nodes == nil || edges == nil || decoder == nil || diagnostics == nil {
		return fail(errors.New("nil ingest port"))
	}
	var e error
	options, e = normalizeOptions(options)
	if e != nil {
		return fail(e)
	}
	b.policy = options.NodePropertyConflictPolicy
	if e = b.load(nodes, edges, decoder, options.Limits); e != nil {
		return fail(e)
	}
	if len(b.nodes) == 0 {
		if e := b.emit(ports.Diagnostic{Severity: ports.Warning, Code: "EMPTY_GRAPH", Message: "no accepted nodes"}); e != nil {
			return fail(e)
		}
	}
	canonicalStart := time.Now()
	b.report.Times.IngestMerge = canonicalStart.Sub(started)
	g, e := b.canonicalize(options.IndexProperties)
	b.report.Times.Canonicalize = time.Since(canonicalStart)
	if e != nil {
		return fail(e)
	}
	b.report.Nodes = g.Metadata().Nodes
	b.report.Edges = g.Metadata().Edges
	return g, b.report, nil
}
func (b *builder) emit(d ports.Diagnostic) error {
	if d.Severity == ports.Rejection {
		b.report.Completeness = Partial
	} else if d.Severity == ports.Warning {
		b.report.Warnings++
	}
	if e := b.ctx.Err(); e != nil {
		return e
	}
	if err := b.sink.Emit(b.ctx, d); err != nil {
		return &DiagnosticError{Cause: err}
	}
	return nil
}
func (b *builder) source(cat ports.Catalog, decoder ports.Decoder, role ports.Role, entry ports.Entry, limits ports.Limits, cr *CatalogReport) (err error) {
	loc := ports.Location{Role: role, Source: entry.Key}
	input, e := cat.Open(b.ctx, entry.Key)
	if e != nil {
		return &ports.SourceError{Kind: ports.SourceIO, Location: loc, Cause: e}
	}
	defer func() {
		e := input.Close()
		if e != nil && err == nil {
			err = &ports.SourceError{Kind: ports.SourceIO, Location: loc, Cause: e}
		}
	}()
	r, e := decoder.New(b.ctx, role, entry, input, limits)
	if e != nil {
		return e
	}
	for {
		if e := b.ctx.Err(); e != nil {
			return e
		}
		ev, e := r.Next(b.ctx)
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		if (ev.Record == nil) == (ev.Rejection == nil) {
			return errors.New("decoder returned invalid event")
		}
		cr.RecordsSeen++
		if ev.Rejection != nil {
			cr.RecordsRejected++
			if e := b.emit(*ev.Rejection); e != nil {
				return e
			}
			continue
		}
		rec := ev.Record
		if rec.Role != role {
			return errors.New("decoder returned wrong role")
		}
		if role == ports.Edges {
			_, src := b.nodes[rec.Source]
			_, dst := b.nodes[rec.Target]
			if b.disk != nil {
				_, src = b.disk.nodes.find(rec.Source)
				_, dst = b.disk.nodes.find(rec.Target)
			}
			if !src || !dst {
				cr.RecordsRejected++
				if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: "ENDPOINT_NOT_FOUND", Location: rec.Location, EntityID: rec.ID, EntityIDKnown: true, Message: "edge endpoint absent"}); e != nil {
					return e
				}
				continue
			}
		}
		for _, d := range ev.Warnings {
			if e := b.emit(d); e != nil {
				return e
			}
		}
		var stageErr error
		if b.disk != nil {
			stageErr = b.disk.stage(rec)
			if stageErr != nil {
				stageErr = &scratchError{stageErr}
			}
		} else {
			stageErr = b.stage(rec)
		}
		if e := stageErr; e != nil {
			return e
		}
		cr.RecordsStaged++
	}
}
func structuralError(s string) error { return fmt.Errorf("invalid normalized record: %s", s) }

// Assign order before sorting; diagnostic locations are opaque and need not
// identify record order. Each property contribution gets its own sequence.
func (b *builder) nextPropertySequence() (uint64, error) {
	if b.sequence == math.MaxUint64 {
		return 0, structuralError("contribution sequence capacity")
	}
	b.sequence++
	return b.sequence, nil
}

// DiagnosticError marks a fatal failure of the diagnostic port.
type DiagnosticError struct{ Cause error }

func (e *DiagnosticError) Error() string { return "diagnostic sink: " + e.Cause.Error() }
func (e *DiagnosticError) Unwrap() error { return e.Cause }

func (b *builder) load(nodes, edges ports.Catalog, decoder ports.Decoder, limits ports.Limits) error {
	lists := [2][]ports.Entry{}
	for role, cat := range []ports.Catalog{nodes, edges} {
		entries, e := cat.List(b.ctx)
		if e != nil {
			return e
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		for i := 1; i < len(entries); i++ {
			if entries[i-1].Key == entries[i].Key {
				return errors.New("duplicate catalog key")
			}
		}
		lists[role] = entries
	}
	for role, cat := range []ports.Catalog{nodes, edges} {
		cr := &b.report.NodeSources
		if role == 1 {
			cr = &b.report.EdgeSources
		}
		for _, entry := range lists[role] {
			if e := b.ctx.Err(); e != nil {
				return e
			}
			cr.SourcesSeen++
			loc := ports.Location{Role: ports.Role(role), Source: entry.Key}
			if !entry.Regular {
				cr.SourcesNonRegular++
				if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: "SOURCE_NON_REGULAR", Location: loc, Message: "catalog entry is not a regular source"}); e != nil {
					return e
				}
				continue
			}
			e := b.source(cat, decoder, ports.Role(role), entry, limits, cr)
			if e == nil {
				cr.SourcesCompleted++
				continue
			}
			if b.ctx.Err() != nil {
				return b.ctx.Err()
			}
			var diagnosticFailure *DiagnosticError
			var scratchFailure *scratchError
			if errors.As(e, &diagnosticFailure) || errors.As(e, &scratchFailure) {
				return e
			}
			var se *ports.SourceError
			if !errors.As(e, &se) {
				return e
			}
			code := "SOURCE_IO"
			switch se.Kind {
			case ports.InvalidHeader:
				cr.SourcesRejectedHeader++
				code = "HEADER_INVALID"
			case ports.SourceIO:
				cr.SourcesIOFailed++
			case ports.UnrecoverableCSV:
				cr.SourcesInterrupted++
				code = "CSV_SOURCE_UNRECOVERABLE"
			default:
				return e
			}
			if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: code, Location: se.Location, Message: "source ended with " + code}); e != nil {
				return e
			}
		}
		// This barrier resolves node properties before any edge source is opened.
		owners := b.nodes
		if role == 1 {
			owners = b.edges
		}
		var err error
		if b.disk != nil {
			err = b.disk.consolidate(ports.Role(role))
		} else {
			err = b.consolidate(ports.Role(role), owners)
		}
		if e := err; e != nil {
			return e
		}
	}
	return nil
}

func normalizeOptions(options Options) (Options, error) {
	if options.NodePropertyConflictPolicy > DropConflictingProperty {
		return options, errors.New("invalid node property conflict policy")
	}
	l := options.Limits
	if l.MaxRecordBytes == 0 {
		l.MaxRecordBytes = 67108864
	}
	if l.MaxColumns == 0 {
		l.MaxColumns = 65536
	}
	if l.MaxRecordBytes < 1024 || l.MaxRecordBytes > uint64(math.MaxInt) || l.MaxColumns > 65536 {
		return options, errors.New("invalid limits")
	}
	options.Limits = l
	for _, key := range options.IndexProperties {
		if key == "" || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
			return options, errors.New("invalid index key")
		}
	}
	return options, nil
}

// A scratch failure must not be reclassified as a recoverable source failure,
// even when a custom storage port wraps a SourceError.
type scratchError struct{ cause error }

func (e *scratchError) Error() string { return "build scratch: " + e.cause.Error() }
func (e *scratchError) Unwrap() error { return e.cause }
