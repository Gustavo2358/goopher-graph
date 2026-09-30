package e2e

import (
	"encoding/json"
	"gophergraph/internal/testutil"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type jsonResult struct {
	Query struct {
		Name           string
		Node, From, To *string
		EdgeLabels     []string
	}
	Counts                    struct{ Nodes, Edges uint64 }
	Directed, PartialSnapshot bool
	Nodes                     []testutil.Node
	Edges                     []testutil.Edge
}

func decodeResult(t *testing.T, data string) jsonResult {
	t.Helper()
	var result jsonResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatal(err, data)
	}
	if !result.Directed || result.Nodes == nil || result.Edges == nil || result.Counts.Nodes != uint64(len(result.Nodes)) || result.Counts.Edges != uint64(len(result.Edges)) {
		t.Fatal(data)
	}
	return result
}
func TestCLIJSONQueries(t *testing.T) {
	bin := binary(t)
	path := filepath.Join(t.TempDir(), "graph.snapshot")
	invoke(t, bin, 0, "build", "--nodes", "../../fixtures/01_topology/nodes", "--edges", "../../fixtures/01_topology/edges", "--output", path)
	expected := testutil.Read(t, "../../fixtures/01_topology")
	for _, q := range expected.Queries {
		command := strings.ReplaceAll(q.Kind, "_", "-")
		args := []string{command, "--snapshot", path, "--format", "json"}
		if command == "between" {
			args = append(args, "--from", q.Start, "--to", q.End)
		} else {
			args = append(args, "--node", q.Start)
		}
		for _, label := range q.Labels {
			args = append(args, "--edge-label", label)
		}
		// CLI expresses an empty effective filter using an unknown label.
		if q.Labels != nil && len(q.Labels) == 0 {
			args = append(args, "--edge-label", "unknown")
		}
		out, diag := invoke(t, bin, 0, args...)
		d := decodeResult(t, out)
		if d.PartialSnapshot || diag != "" || d.Query.Name != command {
			t.Fatal(out, diag)
		}
		if command == "between" {
			if d.Query.Node != nil || d.Query.From == nil || *d.Query.From != q.Start || d.Query.To == nil || *d.Query.To != q.End {
				t.Fatal(d.Query)
			}
		} else if d.Query.Node == nil || *d.Query.Node != q.Start || d.Query.From != nil || d.Query.To != nil {
			t.Fatal(d.Query)
		}
		labels := q.Labels
		if labels != nil && len(labels) == 0 {
			labels = []string{"unknown"}
		}
		if !slices.Equal(d.Query.EdgeLabels, labels) || (d.Query.EdgeLabels == nil) != (labels == nil) {
			t.Fatal(d.Query)
		}
		var nodes, edges []string
		for _, node := range d.Nodes {
			nodes = append(nodes, node.ID)
		}
		for _, edge := range d.Edges {
			edges = append(edges, edge.ID)
			i := slices.IndexFunc(expected.Graph.Edges, func(e testutil.Edge) bool { return e.ID == edge.ID })
			if i < 0 || edge.Source != expected.Graph.Edges[i].Source || edge.Target != expected.Graph.Edges[i].Target {
				t.Fatal(edge)
			}
		}
		if !slices.Equal(nodes, q.Members) || !slices.Equal(edges, q.Edges) {
			t.Fatal(args, nodes, edges)
		}
	}
	base := []string{"territory", "--snapshot", path, "--node", "A", "--format", "json"}
	first, _ := invoke(t, bin, 0, append(slices.Clone(base), "--edge-label", "READS", "--edge-label", "CALLS", "--edge-label", "CALLS")...)
	second, _ := invoke(t, bin, 0, append(slices.Clone(base), "--edge-label", "CALLS", "--edge-label", "READS", "--include-origin")...)
	if first != second {
		t.Fatal("nondeterministic filters or include-origin changed JSON")
	}
	output := filepath.Join(t.TempDir(), "result.json")
	out, _ := invoke(t, bin, 0, append(slices.Clone(base), "--edge-label", "CALLS", "--edge-label", "READS", "--output", output)...)
	saved, err := os.ReadFile(output)
	if err != nil || out != "" || string(saved) != first {
		t.Fatal(out, err)
	}
	invoke(t, bin, 2, append(slices.Clone(base), "--output", path)...)
	invoke(t, bin, 3, "territory", "--snapshot", path, "--node", "missing", "--format", "json")
	invoke(t, bin, 2, "territory", "--snapshot", path, "--node", "A", "--format", "invalid")
	invoke(t, bin, 0, "build", "--nodes", "../../fixtures/02_resilient/nodes", "--edges", "../../fixtures/02_resilient/edges", "--output", path)
	out, diag := invoke(t, bin, 0, base...)
	if !decodeResult(t, out).PartialSnapshot || !strings.Contains(diag, "PARTIAL") {
		t.Fatal(out, diag)
	}
}
