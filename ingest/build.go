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
	"unicode/utf8"
)

type builder struct {
	ctx          context.Context
	sink         ports.DiagnosticSink
	report       Report
	nodes, edges map[string]*entity
}

func Build(ctx context.Context, nodes, edges ports.Catalog, decoder ports.Decoder, diagnostics ports.DiagnosticSink, options Options) (*graph.Graph, Report, error) {
	b := &builder{ctx: ctx, sink: diagnostics, nodes: map[string]*entity{}, edges: map[string]*entity{}}
	fail := func(e error) (*graph.Graph, Report, error) { return nil, b.report, e }
	if e := ctx.Err(); e != nil {
		return fail(e)
	}
	if nodes == nil || edges == nil || decoder == nil || diagnostics == nil {
		return fail(errors.New("nil ingest port"))
	}
	l := options.Limits
	if l.MaxRecordBytes == 0 {
		l.MaxRecordBytes = 67108864
	}
	if l.MaxColumns == 0 {
		l.MaxColumns = 65536
	}
	if l.MaxRecordBytes < 1024 || l.MaxRecordBytes > uint64(math.MaxInt) || l.MaxColumns > 65536 {
		return fail(errors.New("invalid limits"))
	}
	options.Limits = l
	for _, key := range options.IndexProperties {
		if key == "" || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
			return fail(errors.New("invalid index key"))
		}
	}
	lists := [2][]ports.Entry{}
	for role, cat := range []ports.Catalog{nodes, edges} {
		entries, e := cat.List(ctx)
		if e != nil {
			return fail(e)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		for i := 1; i < len(entries); i++ {
			if entries[i-1].Key == entries[i].Key {
				return fail(errors.New("duplicate catalog key"))
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
			if e := ctx.Err(); e != nil {
				return fail(e)
			}
			cr.SourcesSeen++
			loc := ports.Location{Role: ports.Role(role), Source: entry.Key}
			if !entry.Regular {
				cr.SourcesNonRegular++
				if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: "SOURCE_NON_REGULAR", Location: loc, Message: "catalog entry is not a regular source"}); e != nil {
					return fail(e)
				}
				continue
			}
			e := b.source(cat, decoder, ports.Role(role), entry, l, cr)
			if e == nil {
				cr.SourcesCompleted++
				continue
			}
			if ctx.Err() != nil {
				return fail(ctx.Err())
			}
			var diagnosticFailure *DiagnosticError
			if errors.As(e, &diagnosticFailure) {
				return fail(e)
			}
			var se *ports.SourceError
			if !errors.As(e, &se) {
				return fail(e)
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
				return fail(e)
			}
			if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: code, Location: se.Location, Message: "source ended with " + code}); e != nil {
				return fail(e)
			}
		}
		// This barrier resolves node properties before any edge source is opened.
		owners := b.nodes
		if role == 1 {
			owners = b.edges
		}
		if e := b.consolidate(owners); e != nil {
			return fail(e)
		}
	}
	if len(b.nodes) == 0 {
		if e := b.emit(ports.Diagnostic{Severity: ports.Warning, Code: "EMPTY_GRAPH", Message: "no accepted nodes"}); e != nil {
			return fail(e)
		}
	}
	g, e := b.canonicalize(options.IndexProperties)
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
		if e := b.stage(rec); e != nil {
			return e
		}
		cr.RecordsStaged++
	}
}
func structuralError(s string) error { return fmt.Errorf("invalid normalized record: %s", s) }

// DiagnosticError marks a fatal failure of the diagnostic port.
type DiagnosticError struct{ Cause error }

func (e *DiagnosticError) Error() string { return "diagnostic sink: " + e.Cause.Error() }
func (e *DiagnosticError) Unwrap() error { return e.Cause }
