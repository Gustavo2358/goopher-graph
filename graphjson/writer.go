// Package graphjson streams directed query subgraphs as typed JSON.
package graphjson

import (
	"bufio"
	"context"
	"encoding/json"
	"gophergraph/graph"
	"gophergraph/query"
	"io"
	"math"
	"slices"
	"strconv"
)

// Query describes the query that produced the subgraph. Identifiers and labels
// are external strings. Nil endpoint pointers omit fields; a pointer to "" is
// a valid empty ID. EdgeLabels nil means unrestricted, empty means no labels.
// The caller supplies the executed parameters; Write does not rerun the query.
type Query struct {
	Name       string   `json:"name"`
	Node       *string  `json:"node,omitempty"`
	From       *string  `json:"from,omitempty"`
	To         *string  `json:"to,omitempty"`
	EdgeLabels []string `json:"edgeLabels"`
}

// Write emits one JSON object followed by a newline. It includes every member
// of s, with external IDs, original edge direction and typed property entries.
// Long values are decimal strings; nonfinite floats use "NaN", "+Inf", "-Inf".
// Memory is bounded by the largest scalar, one node's labels and query metadata,
// in addition to g and s. The caller keeps g open and s unchanged until return.
// Cancellation or an output error may leave a JSON prefix; discard that output.
// Write flushes its own buffer but does not close output.
func Write(ctx context.Context, output io.Writer, g *graph.Graph, s *query.Subgraph, q Query) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.BelongsTo(g) {
		return graph.ErrGraphMismatch
	}
	if _, _, err := g.FindString(""); err != nil {
		return err
	}
	q.EdgeLabels = slices.Clone(q.EdgeLabels)
	slices.Sort(q.EdgeLabels)
	q.EdgeLabels = slices.Compact(q.EdgeLabels)
	w := writer{ctx: ctx, out: bufio.NewWriter(output)}
	w.raw(`{"query":`)
	w.json(q)
	w.raw(`,"directed":true,"partialSnapshot":`)
	w.raw(strconv.FormatBool(g.Metadata().PartialLoad))
	w.raw(`,"counts":{"nodes":`)
	w.raw(strconv.FormatUint(s.NodeCount(), 10))
	w.raw(`,"edges":`)
	w.raw(strconv.FormatUint(s.EdgeCount(), 10))
	w.raw(`},"nodes":[`)
	nodes := s.Nodes()
	sep := ""
	for w.err == nil && nodes.Next() {
		w.raw(sep)
		sep = ","
		w.node(g, nodes.ID())
	}
	w.raw(`],"edges":[`)
	edges := s.Edges()
	sep = ""
	for w.err == nil && edges.Next() {
		w.raw(sep)
		sep = ","
		w.edge(g, edges.ID())
	}
	w.raw("]}\n")
	if w.err != nil {
		return w.err
	}
	return w.out.Flush()
}

// Sticky errors stop iteration as soon as a write, graph access or context fails.
type writer struct {
	ctx context.Context
	out *bufio.Writer
	err error
}

func (w *writer) raw(s string) {
	if w.err != nil {
		return
	}
	if w.err = w.ctx.Err(); w.err == nil {
		_, w.err = w.out.WriteString(s)
	}
}
func (w *writer) json(v any) {
	if w.err != nil {
		return
	}
	var b []byte
	b, w.err = json.Marshal(v)
	if w.err == nil {
		_, w.err = w.out.Write(b)
	}
}
func (w *writer) text(s string, err error) {
	if w.err != nil {
		return
	}
	if err != nil {
		w.err = err
		return
	}
	w.json(s)
}
func (w *writer) node(g *graph.Graph, id graph.NodeID) {
	w.raw(`{"id":`)
	w.text(g.NodeExternalID(id))
	w.raw(`,"labels":[`)
	labels, err := g.NodeLabels(id)
	if err != nil {
		if w.err == nil {
			w.err = err
		}
		return
	}
	for i, label := range labels {
		if w.err != nil {
			return
		}
		if i != 0 {
			w.raw(",")
		}
		w.text(g.String(label))
	}
	w.raw(`],"properties":[`)
	it, err := g.NodeProperties(id)
	w.properties(g, it, err)
	w.raw(`]}`)
}

func (w *writer) edge(g *graph.Graph, id graph.EdgeID) {
	if w.err != nil {
		return
	}
	edge, err := g.Edge(id)
	if err != nil {
		w.err = err
		return
	}
	w.raw(`{"id":`)
	w.text(g.EdgeExternalID(id))
	w.raw(`,"source":`)
	w.text(g.NodeExternalID(edge.Source))
	w.raw(`,"target":`)
	w.text(g.NodeExternalID(edge.Target))
	w.raw(`,"label":`)
	w.text(g.String(edge.Label))
	w.raw(`,"properties":[`)
	it, err := g.EdgeProperties(id)
	w.properties(g, it, err)
	w.raw(`]}`)
}
func (w *writer) properties(g *graph.Graph, it graph.PropertyIterator, err error) {
	if w.err != nil {
		return
	}
	if err != nil {
		w.err = err
		return
	}
	sep := ""
	for w.err == nil && it.Next() {
		p := it.Property()
		w.raw(sep)
		sep = ","
		w.raw(`{"key":`)
		w.text(g.String(p.Key))
		w.raw(`,"type":`)
		// Graph validation guarantees one of these ten kinds.
		names := [...]string{"", "Bool", "Byte", "Short", "Int", "Long", "Float", "Double", "String", "Date", "Datetime"}
		if p.Value.Kind() < graph.BoolKind || p.Value.Kind() > graph.DatetimeKind {
			w.err = graph.ErrInvalidValue
			return
		}
		w.json(names[p.Value.Kind()])
		w.raw(`,"value":`)
		w.value(p.Value)
		w.raw(`}`)
	}
	if w.err == nil {
		w.err = it.Err()
	}
}
func (w *writer) value(v graph.Value) {
	switch v.Kind() {
	case graph.BoolKind:
		b, _ := v.Bool()
		w.raw(strconv.FormatBool(b))
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		n, _ := v.Int64()
		s := strconv.FormatInt(n, 10)
		if v.Kind() == graph.LongKind {
			w.json(s)
		} else {
			w.raw(s)
		}
	case graph.FloatKind, graph.DoubleKind:
		f, _ := v.Float64()
		switch {
		case math.IsNaN(f):
			w.raw(`"NaN"`)
		case math.IsInf(f, 1):
			w.raw(`"+Inf"`)
		case math.IsInf(f, -1):
			w.raw(`"-Inf"`)
		default:
			bits := 64
			if v.Kind() == graph.FloatKind {
				bits = 32
			}
			w.raw(strconv.FormatFloat(f, 'g', -1, bits))
		}
	case graph.StringKind, graph.DateKind, graph.DatetimeKind:
		s, _ := v.Text()
		w.json(s)
	}
}
