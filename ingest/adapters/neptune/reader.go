// Package neptune decodes the documented Gremlin CSV profile incrementally.
package neptune

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"gophergraph/ingest/ports"
	"io"
	"math"
	"strconv"
	"unicode/utf8"
)

type Decoder struct{}
type reader struct {
	f     framer
	cols  []column
	role  ports.Role
	entry ports.Entry
	done  bool
}

func NormalizeLimits(l ports.Limits) (ports.Limits, error) {
	if l.MaxRecordBytes == 0 {
		l.MaxRecordBytes = 67108864
	}
	if l.MaxColumns == 0 {
		l.MaxColumns = 65536
	}
	if l.MaxRecordBytes < 1024 || l.MaxRecordBytes > uint64(math.MaxInt) || l.MaxColumns > 65536 {
		return l, errors.New("invalid decoder limits")
	}
	return l, nil
}
func (Decoder) New(ctx context.Context, role ports.Role, entry ports.Entry, input io.Reader, limits ports.Limits) (ports.RecordReader, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	l, e := NormalizeLimits(limits)
	if e != nil {
		return nil, e
	}
	if role > ports.Edges {
		return nil, errors.New("invalid catalog role")
	}
	r := &reader{f: framer{r: bufio.NewReader(input), limits: l, line: 1}, role: role, entry: entry}
	raw, line, e := r.f.frame(ctx)
	loc := ports.Location{Role: role, Source: entry.Key, Record: 1, Line: line}
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		kind := ports.InvalidHeader
		if !errors.Is(e, io.EOF) && !errors.Is(e, errLexical) && !errors.Is(e, errLimit) {
			kind = ports.SourceIO
		}
		return nil, &ports.SourceError{Kind: kind, Location: loc, Cause: e}
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, &ports.SourceError{Kind: ports.InvalidHeader, Location: loc, Cause: errors.New("invalid header encoding")}
	}
	fields, e := cells(raw)
	if e == nil {
		r.cols, e = header(fields, role)
	}
	if e != nil {
		return nil, &ports.SourceError{Kind: ports.InvalidHeader, Location: loc, Cause: e}
	}
	return r, nil
}
func (r *reader) Next(ctx context.Context) (ports.Event, error) {
	if e := ctx.Err(); e != nil {
		return ports.Event{}, e
	}
	if r.done {
		return ports.Event{}, io.EOF
	}
	raw, line, e := r.f.frame(ctx)
	loc := ports.Location{Role: r.role, Source: r.entry.Key, Record: r.f.record, Line: line}
	reject := func(code, message string) ports.Event {
		return ports.Event{Rejection: &ports.Diagnostic{Severity: ports.Rejection, Code: code, Location: loc, Message: message}}
	}
	if errors.Is(e, errLimit) {
		return reject("RECORD_LIMIT", "record exceeds configured limits"), nil
	}
	if e != nil {
		r.done = true
		if errors.Is(e, io.EOF) {
			return ports.Event{}, io.EOF
		}
		if ctx.Err() != nil {
			return ports.Event{}, ctx.Err()
		}
		kind := ports.SourceIO
		if errors.Is(e, errLexical) {
			kind = ports.UnrecoverableCSV
		}
		return ports.Event{}, &ports.SourceError{Kind: kind, Location: loc, Cause: e}
	}
	if !utf8.Valid(raw) {
		return reject("UTF8_INVALID", "record is not valid UTF-8"), nil
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		return reject("VALUE_INVALID", "record contains NUL"), nil
	}
	fields, e := cells(raw)
	if e != nil {
		return reject("VALUE_INVALID", "invalid CSV record"), nil
	}
	if len(fields) != len(r.cols) {
		return reject("VALUE_INVALID", "column count differs from header"), nil
	}
	rec := &ports.Record{Role: r.role, Location: loc}
	var warnings []ports.Diagnostic
	labelPresent := false
	for i, col := range r.cols {
		c := fields[i]
		loc.Column = col.name
		if col.reserved {
			switch col.name {
			case "~id", "~from", "~to":
				if !c.present {
					return reject("REQUIRED_FIELD_MISSING", "required field absent"), nil
				}
				switch col.name {
				case "~id":
					rec.ID = c.text
				case "~from":
					rec.Source = c.text
				case "~to":
					rec.Target = c.text
				}
			case "~label":
				if c.present {
					labelPresent = true
					labels := segments(c.text, false)
					if r.role == ports.Edges && len(labels) != 1 {
						return reject("VALUE_INVALID", "edge needs one label"), nil
					}
					for _, label := range labels {
						if label == "" {
							return reject("VALUE_INVALID", "label segment empty"), nil
						}
					}
					rec.Labels = labels
				}
			}
			continue
		}
		if !c.present {
			continue
		}
		values := []string{c.text}
		if col.array {
			values = segments(c.text, col.kind == 8)
		}
		for _, s := range values {
			v, coerced, e := scalar(col.kind, s)
			if e != nil {
				return reject("VALUE_INVALID", "invalid typed value in column "+strconv.Quote(col.name)), nil
			}
			rec.Properties = append(rec.Properties, ports.Property{Key: col.name, Value: v, Cardinality: col.cardinality})
			if coerced {
				warnings = append(warnings, ports.Diagnostic{Severity: ports.Warning, Code: "BOOL_COERCED", Location: loc, EntityID: rec.ID, EntityIDKnown: true, Message: "non-boolean token coerced to false"})
			}
		}
	}
	if !labelPresent {
		label := "vertex"
		if r.role == ports.Edges {
			label = "edge"
		}
		rec.Labels = []string{label}
	}
	for i := range warnings {
		warnings[i].EntityID = rec.ID
	}
	return ports.Event{Record: rec, Warnings: warnings}, nil
}
