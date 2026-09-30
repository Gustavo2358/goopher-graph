package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"io"
	"math"
	"sort"
	"time"
	"unicode/utf8"
)

type diskBuilder struct {
	b                *builder
	ws               ports.Workspace
	budget           uint64
	pending, strings *spillFile
	accepted         [2]*spillFile
	counts           [2]uint64
	nodes            diskDictionary
	nodeMaps         []ports.Mapping
}
type diskDictionary struct{ offsets, data []byte }

func (d diskDictionary) count() int { return len(d.offsets)/8 - 1 }
func (d diskDictionary) at(i int) []byte {
	return d.data[spillLE.Uint64(d.offsets[i*8:]):spillLE.Uint64(d.offsets[(i+1)*8:])]
}
func (d diskDictionary) find(s string) (uint32, bool) {
	n := d.count()
	i := sort.Search(n, func(i int) bool { return bytes.Compare(d.at(i), []byte(s)) >= 0 })
	return uint32(i), i < n && bytes.Equal(d.at(i), []byte(s))
}
func closeMappings(m []ports.Mapping) error {
	var err error
	for _, b := range m {
		err = errors.Join(err, b.Close())
	}
	return err
}
func mapSpill(ctx context.Context, ws ports.Workspace, f *spillFile, m *[]ports.Mapping) ([]byte, error) {
	if err := f.flush(); err != nil {
		return nil, err
	}
	if f.size == 0 {
		return []byte{}, nil
	}
	b, err := ws.Map(ctx, f.file)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("scratch returned nil mapping")
	}
	*m = append(*m, b)
	if int64(len(b.Bytes())) != f.size {
		return nil, errors.New("scratch mapping size mismatch")
	}
	return b.Bytes(), nil
}
func buildDisk(ctx context.Context, nodes, edges ports.Catalog, decoder ports.Decoder, diag ports.DiagnosticSink, options Options) (g *graph.Graph, report Report, err error) {
	started := time.Now()
	b := &builder{ctx: ctx, sink: diag}
	defer func() { report = b.report }()
	if err = ctx.Err(); err != nil {
		return
	}
	if nodes == nil || edges == nil || decoder == nil || diag == nil {
		err = errors.New("nil ingest port")
		return
	}
	options, err = normalizeOptions(options)
	if err != nil {
		return
	}
	budget := options.MemoryBudget
	if budget == 0 {
		budget = 64 << 20
	}
	if budget < 1<<20 || budget > uint64(math.MaxInt) {
		err = errors.New("memory budget must be 1 MiB..MaxInt")
		return
	}
	ws, err := options.Scratch.New(ctx)
	if err != nil {
		return
	}
	if ws == nil {
		return nil, Report{}, errors.New("scratch returned nil workspace")
	}
	d := &diskBuilder{b: b, ws: ws, budget: budget}
	b.disk = d
	defer func() {
		err = errors.Join(err, closeMappings(d.nodeMaps), ws.Close())
		if err != nil && g != nil {
			err = errors.Join(err, g.Close())
			g = nil
		}
	}()
	d.pending, err = newSpill(ws)
	if err != nil {
		return
	}
	d.strings, err = newSpill(ws)
	if err != nil {
		return
	}
	if err = d.strings.row(nil, nil); err != nil {
		return
	}
	for _, key := range options.IndexProperties {
		if err = d.strings.row([]byte(key), nil); err != nil {
			return
		}
	}
	if err = b.load(nodes, edges, decoder, options.Limits); err != nil {
		return
	}
	if d.counts[0] == 0 {
		if err = b.emit(ports.Diagnostic{Severity: ports.Warning, Code: "EMPTY_GRAPH", Message: "no accepted nodes"}); err != nil {
			return
		}
	}
	canonicalStart := time.Now()
	b.report.Times.IngestMerge = canonicalStart.Sub(started)
	g, err = d.canonicalize(options.IndexProperties)
	b.report.Times.Canonicalize = time.Since(canonicalStart)
	if err == nil {
		b.report.Nodes = d.counts[0]
		b.report.Edges = d.counts[1]
	}
	return
}
func originBytes(loc ports.Location) []byte {
	b := sortString(nil, loc.Source)
	b = big64(b, loc.Record)
	b = big64(b, loc.Line)
	return sortString(b, loc.Column)
}
func readOrigin(b []byte, role ports.Role) (ports.Location, error) {
	source, rest, err := takeString(b)
	if err != nil {
		return ports.Location{}, err
	}
	if len(rest) < 16 {
		return ports.Location{}, io.ErrUnexpectedEOF
	}
	loc := ports.Location{Role: role, Source: source, Record: spillBE.Uint64(rest), Line: spillBE.Uint64(rest[8:])}
	loc.Column, rest, err = takeString(rest[16:])
	if err == nil && len(rest) != 0 {
		err = errors.New("invalid scratch origin")
	}
	return loc, err
}
func (d *diskBuilder) stage(r *ports.Record) error {
	if !utf8.ValidString(r.ID) || len(r.Labels) == 0 {
		return structuralError("identity/labels")
	}
	for _, label := range r.Labels {
		if label == "" || !utf8.ValidString(label) {
			return structuralError("label")
		}
	}
	if r.Role == ports.Edges && len(r.Labels) != 1 {
		return structuralError("edge labels")
	}
	base := sortString(nil, r.ID)
	key := append(bytes.Clone(base), 0)
	if r.Role == ports.Edges {
		key = sortString(key, r.Source)
		key = sortString(key, r.Target)
		key = sortString(key, r.Labels[0])
	}
	loc := originBytes(r.Location)
	if err := d.pending.row(key, loc); err != nil {
		return err
	}
	for _, label := range r.Labels {
		if err := d.pending.row(sortString(append(bytes.Clone(base), 1), label), nil); err != nil {
			return err
		}
	}
	for _, p := range r.Properties {
		if p.Key == "" || !utf8.ValidString(p.Key) || p.Cardinality > ports.Single || p.Value.Kind() < graph.BoolKind || p.Value.Kind() > graph.DatetimeKind || (r.Role == ports.Edges && p.Cardinality != ports.Single) {
			return structuralError("property")
		}
		key := sortString(append(bytes.Clone(base), 2), p.Key)
		key = append(key, byte(p.Value.Kind()))
		if s, ok := p.Value.Text(); ok {
			key = sortString(key, s)
		} else {
			key = big64(key, payload(p.Value, nil))
		}
		if err := d.pending.row(key, append([]byte{byte(p.Cardinality)}, loc...)); err != nil {
			return err
		}
	}
	return nil
}
func token(k []byte) (id string, tag byte, rest []byte, err error) {
	id, rest, err = takeString(k)
	if err != nil {
		return
	}
	if len(rest) == 0 || rest[0] > 2 {
		err = errors.New("invalid scratch token")
		return
	}
	tag, rest = rest[0], rest[1:]
	return
}
func (d *diskBuilder) accept(out *spillFile, key []byte) error {
	id, tag, rest, err := token(key)
	if err != nil {
		return err
	}
	if err = out.row(key, nil); err != nil {
		return err
	}
	add := func(s string) error { return d.strings.row([]byte(s), nil) }
	switch tag {
	case 0:
		if err = add(id); err != nil {
			return err
		}
		// Edge endpoints already occur in the accepted node dictionary.
		if len(rest) > 0 {
			_, rest, err = takeString(rest)
			if err != nil {
				return err
			}
			_, rest, err = takeString(rest)
			if err != nil {
				return err
			}
			var label string
			label, _, err = takeString(rest)
			if err != nil {
				return err
			}
			return add(label)
		}
	case 1:
		var label string
		label, _, err = takeString(rest)
		if err != nil {
			return err
		}
		return add(label)
	case 2:
		var key string
		key, rest, err = takeString(rest)
		if err != nil {
			return err
		}
		if err = add(key); err != nil {
			return err
		}
		if len(rest) == 0 {
			return io.ErrUnexpectedEOF
		}
		if rest[0] >= byte(graph.StringKind) {
			var text string
			text, _, err = takeString(rest[1:])
			if err != nil {
				return err
			}
			return add(text)
		}
	}
	return nil
}
func (d *diskBuilder) consolidate(role ports.Role) error {
	sorted, err := externalSort(d.b.ctx, d.ws, d.pending, d.budget)
	if err != nil {
		return err
	}
	out, err := newSpill(d.ws)
	if err != nil {
		return err
	}
	d.accepted[role] = out
	var nodeOff, nodeText *spillFile
	if role == ports.Nodes {
		nodeOff, err = newSpill(d.ws)
		if err != nil {
			return err
		}
		nodeText, err = newSpill(d.ws)
		if err != nil {
			return err
		}
	}
	r := readSpill(sorted, 0, sorted.size)
	groupReader := readSpill(sorted, 0, 0)
	nextErr := r.next()
	quarantined := false
	for nextErr == nil {
		if err = d.b.ctx.Err(); err != nil {
			return err
		}
		id, tag, rest, err := token(r.row.key)
		if err != nil {
			return err
		}
		switch tag {
		case 0:
			prefix := append(sortString(nil, id), 0)
			first := bytes.Clone(r.row.key)
			var origins []ports.Location
			var count uint64
			quarantined = false
			for nextErr == nil && bytes.HasPrefix(r.row.key, prefix) {
				loc, err := readOrigin(r.row.value, role)
				if err != nil {
					return err
				}
				count++
				origins = addOrigin(origins, loc)
				if !bytes.Equal(first, r.row.key) {
					quarantined = true
				}
				nextErr = r.next()
				if err = d.b.ctx.Err(); err != nil {
					return err
				}
			}
			if quarantined {
				d.b.report.QuarantinedEdgeIDs++
				if err = d.b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: "EDGE_ID_CONFLICT", Location: origins[0], Related: origins, Contributions: count, EntityID: id, EntityIDKnown: true, Message: "inconsistent edge structure"}); err != nil {
					return err
				}
			} else {
				if d.counts[role] >= math.MaxUint32 {
					return structuralError("entity capacity")
				}
				d.counts[role]++
				if err = d.accept(out, first); err != nil {
					return err
				}
				if role == ports.Nodes {
					if err = nodeOff.u64(uint64(nodeText.size)); err != nil {
						return err
					}
					if err = nodeText.write([]byte(id)); err != nil {
						return err
					}
				}
			}
		case 1:
			key := bytes.Clone(r.row.key)
			if !quarantined {
				if err = d.accept(out, key); err != nil {
					return err
				}
			}
			for nextErr == nil && bytes.Equal(key, r.row.key) {
				nextErr = r.next()
				if err = d.b.ctx.Err(); err != nil {
					return err
				}
			}
		case 2:
			name, values, err := takeString(rest)
			if err != nil {
				return err
			}
			prefix := bytes.Clone(r.row.key[:len(r.row.key)-len(values)])
			start, end := r.start, r.start
			var origins []ports.Location
			var count uint64
			var cards uint8
			var prev []byte
			distinct := 0
			for nextErr == nil && bytes.HasPrefix(r.row.key, prefix) {
				if len(r.row.value) == 0 {
					return io.ErrUnexpectedEOF
				}
				cards |= 1 << r.row.value[0]
				count++
				loc, err := readOrigin(r.row.value[1:], role)
				if err != nil {
					return err
				}
				loc.Column = name
				origins = addOrigin(origins, loc)
				if prev == nil || !bytes.Equal(prev, r.row.key) {
					distinct = min(2, distinct+1)
					prev = append(prev[:0], r.row.key...)
				}
				end = r.end
				nextErr = r.next()
				if err = d.b.ctx.Err(); err != nil {
					return err
				}
			}
			if quarantined {
				continue
			}
			code := ""
			if cards == 3 {
				code = "PROPERTY_CARDINALITY_CONFLICT"
				d.b.report.PropertyCardinalityConflictGroups++
			} else if cards == 2 && distinct > 1 {
				code = "PROPERTY_CONFLICT"
				d.b.report.PropertyConflictGroups++
			}
			if code != "" {
				if err = d.b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: code, Location: origins[0], Related: origins, Contributions: count, EntityID: id, EntityIDKnown: true, Message: fmt.Sprintf("property removed after %d contributions", count)}); err != nil {
					return err
				}
			} else {
				// Re-read a property group after its decision. Even one enormous set stays
				// on disk; only the previous value and two diagnostic origins are retained.
				groupReader.reset(sorted, start, end)
				prev = nil
				for {
					if err = d.b.ctx.Err(); err != nil {
						return err
					}
					err = groupReader.next()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						return err
					}
					if prev == nil || !bytes.Equal(prev, groupReader.row.key) {
						if err = d.accept(out, groupReader.row.key); err != nil {
							return err
						}
						prev = append(prev[:0], groupReader.row.key...)
					}
				}
			}
		}
	}
	if !errors.Is(nextErr, io.EOF) {
		return nextErr
	}
	if err = out.flush(); err != nil {
		return err
	}
	if err = d.ws.Remove(sorted.file); err != nil {
		return err
	}
	if role == ports.Nodes {
		if err = nodeOff.u64(uint64(nodeText.size)); err != nil {
			return err
		}
		d.nodes.offsets, err = mapSpill(d.b.ctx, d.ws, nodeOff, &d.nodeMaps)
		if err != nil {
			return err
		}
		d.nodes.data, err = mapSpill(d.b.ctx, d.ws, nodeText, &d.nodeMaps)
		if err != nil {
			return err
		}
		d.pending, err = newSpill(d.ws)
	}
	return err
}
