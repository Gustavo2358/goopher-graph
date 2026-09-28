package ingest

import (
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

type group struct {
	cards   uint8
	values  map[graph.Value]struct{}
	count   uint64
	origins []ports.Location
}
type entity struct {
	id, source, target, label string
	labels                    map[string]bool
	props                     map[string]*group
	quarantined               bool
	count                     uint64
	origins                   []ports.Location
}

func addOrigin(list []ports.Location, loc ports.Location) []ports.Location {
	for _, v := range list {
		if v == loc {
			return list
		}
	}
	list = append(list, loc)
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Record != b.Record {
			return a.Record < b.Record
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	if len(list) > 2 {
		list = list[:2]
	}
	return list
}
func cloneValue(v graph.Value) graph.Value {
	if s, ok := v.Text(); ok {
		v, _ = graph.TextValue(v.Kind(), strings.Clone(s))
	}
	return v
}
func (b *builder) stage(r *ports.Record) error {
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
	owners := b.nodes
	if r.Role == ports.Edges {
		owners = b.edges
	}
	e := owners[r.ID]
	if e == nil {
		if uint64(len(owners)) >= math.MaxUint32 {
			return structuralError("entity capacity")
		}
		e = &entity{id: strings.Clone(r.ID), source: strings.Clone(r.Source), target: strings.Clone(r.Target), label: strings.Clone(r.Labels[0]), labels: map[string]bool{}, props: map[string]*group{}}
		owners[e.id] = e
	}
	e.count++
	e.origins = addOrigin(e.origins, r.Location)
	if r.Role == ports.Edges && (e.source != r.Source || e.target != r.Target || e.label != r.Labels[0]) {
		e.quarantined = true
	}
	if e.quarantined {
		return nil
	}
	for _, label := range r.Labels {
		e.labels[strings.Clone(label)] = true
	}
	for _, p := range r.Properties {
		if p.Key == "" || !utf8.ValidString(p.Key) || p.Cardinality > ports.Single || p.Value.Kind() < graph.BoolKind || p.Value.Kind() > graph.DatetimeKind || (r.Role == ports.Edges && p.Cardinality != ports.Single) {
			return structuralError("property")
		}
		g := e.props[p.Key]
		if g == nil {
			g = &group{values: map[graph.Value]struct{}{}}
			e.props[strings.Clone(p.Key)] = g
		}
		g.cards |= 1 << p.Cardinality
		g.values[cloneValue(p.Value)] = struct{}{}
		g.count++
		loc := r.Location
		loc.Column = p.Key
		g.origins = addOrigin(g.origins, loc)
	}
	return nil
}
func sortedEntities(m map[string]*entity) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func (b *builder) consolidate(owners map[string]*entity) error {
	for _, id := range sortedEntities(owners) {
		if e := b.ctx.Err(); e != nil {
			return e
		}
		entity := owners[id]
		if entity.quarantined {
			b.report.QuarantinedEdgeIDs++
			if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: "EDGE_ID_CONFLICT", Location: entity.origins[0], Related: entity.origins, Contributions: entity.count, EntityID: id, EntityIDKnown: true, Message: "inconsistent edge structure"}); e != nil {
				return e
			}
			delete(owners, id)
			continue
		}
		keys := make([]string, 0, len(entity.props))
		for key := range entity.props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			g := entity.props[key]
			code := ""
			if g.cards == 3 {
				code = "PROPERTY_CARDINALITY_CONFLICT"
				b.report.PropertyCardinalityConflictGroups++
			} else if g.cards == 2 && len(g.values) > 1 {
				code = "PROPERTY_CONFLICT"
				b.report.PropertyConflictGroups++
			}
			if code != "" {
				if e := b.emit(ports.Diagnostic{Severity: ports.Rejection, Code: code, Location: g.origins[0], Related: g.origins, Contributions: g.count, EntityID: id, EntityIDKnown: true, Message: fmt.Sprintf("property removed after %d contributions", g.count)}); e != nil {
					return e
				}
				delete(entity.props, key)
			}
		}
	}
	return nil
}
