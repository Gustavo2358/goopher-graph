// Package dot exports directed multigraphs as escaped Graphviz DOT text.
package dot

import (
	"bufio"
	"fmt"
	"gophergraph/graph"
	"gophergraph/query"
	"io"
	"strings"
)

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		default:
			if r < 32 || r == 127 {
				fmt.Fprintf(&b, "\\\\x%02X", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
func Write(output io.Writer, g *graph.Graph, s *query.Subgraph) error {
	if !s.BelongsTo(g) {
		return graph.ErrGraphMismatch
	}
	w := bufio.NewWriter(output)
	if _, e := w.WriteString("digraph G {\n"); e != nil {
		return e
	}
	nodes := s.Nodes()
	for nodes.Next() {
		id := nodes.ID()
		external, e := g.NodeExternalID(id)
		if e != nil {
			return e
		}
		labels, e := g.NodeLabels(id)
		if e != nil {
			return e
		}
		names := make([]string, len(labels))
		for i, l := range labels {
			names[i], e = g.String(l)
			if e != nil {
				return e
			}
		}
		if _, e = fmt.Fprintf(w, "  n%d [label=%s];\n", id, quote(external+"\n"+strings.Join(names, ";"))); e != nil {
			return e
		}
	}
	edges := s.Edges()
	for edges.Next() {
		edge, e := g.Edge(edges.ID())
		if e != nil {
			return e
		}
		label, e := g.String(edge.Label)
		if e != nil {
			return e
		}
		external, e := g.EdgeExternalID(edge.ID)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintf(w, "  n%d -> n%d [label=%s, id=\"e%d\", tooltip=%s];\n", edge.Source, edge.Target, quote(label), edge.ID, quote(external)); e != nil {
			return e
		}
	}
	if _, e := w.WriteString("}\n"); e != nil {
		return e
	}
	return w.Flush()
}
