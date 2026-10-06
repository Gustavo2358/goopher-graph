package e2e

import (
	"gophergraph/internal/testutil"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCLIGGPB(t *testing.T) {
	bin := binary(t)
	for _, fixture := range []string{"01_topology", "02_resilient", "05_defaults_empty_id"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "graph.snapshot")
		out := filepath.Join(dir, "result.ggpb")
		invoke(t, bin, 0, "build", "--nodes", "../../fixtures/"+fixture+"/nodes", "--edges", "../../fixtures/"+fixture+"/edges", "--output", path)
		for _, q := range testutil.Read(t, "../../fixtures/"+fixture).Queries {
			command := strings.ReplaceAll(q.Kind, "_", "-")
			args := []string{command, "--snapshot", path}
			if command == "between" {
				args = append(args, "--from", q.Start, "--to", q.End)
			} else {
				args = append(args, "--node", q.Start)
			}
			for _, l := range q.Labels {
				args = append(args, "--edge-label", l)
			}
			if q.Labels != nil && len(q.Labels) == 0 {
				args = append(args, "--edge-label", "unknown")
			}
			want, diag := invoke(t, bin, 0, append(slices.Clone(args), "--format", "json")...)
			stdout, ggdiag := invoke(t, bin, 0, append(slices.Clone(args), "--format", "ggpb", "--output", out)...)
			if stdout != "" || diag != ggdiag {
				t.Fatal(stdout, diag, ggdiag)
			}
			got, _ := invoke(t, bin, 0, "decode", "--input", out, "--format", "json")
			if want != got {
				t.Fatal("CLI parity", want, got)
			}
			first, e := os.ReadFile(out)
			if e != nil {
				t.Fatal(e)
			}
			invoke(t, bin, 0, append(slices.Clone(args), "--format", "ggpb", "--output", out, "--include-origin")...)
			second, e := os.ReadFile(out)
			if e != nil || string(first) != string(second) {
				t.Fatal("determinism", e)
			}
			raw, _ := invoke(t, bin, 0, append(slices.Clone(args), "--format", "ggpb")...)
			if raw != string(first) {
				t.Fatal("stdout")
			}
			invoke(t, bin, 2, "decode", "--input", out, "--output", out)
			if e := os.WriteFile(out, first[:len(first)-1], 0600); e != nil {
				t.Fatal(e)
			}
			invoke(t, bin, 1, "decode", "--input", out)
		}
	}
	invoke(t, bin, 2, "decode", "--format", "json")
}
