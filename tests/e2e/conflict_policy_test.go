package e2e

import (
	"encoding/json"
	"gophergraph/ingest/ports"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLINodePropertyConflictPolicies(t *testing.T) {
	bin := binary(t)
	dir := t.TempDir()
	nodes, edges := filepath.Join(dir, "nodes"), filepath.Join(dir, "edges")
	for _, path := range []string{nodes, edges} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nodes, "n.csv"), []byte("~id,sigla:String(single)\nA,ZZ\nA,AA\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "graph.snapshot")
	base := []string{"build", "--nodes", nodes, "--edges", edges, "--output", path, "--memory-budget", "1048576", "--index-property", "sigla"}
	for _, tc := range []struct {
		name, flag, winner string
		partial            bool
	}{
		{name: "default", winner: "AA"},
		{name: "last-wins", flag: "last-wins", winner: "AA"},
		{name: "first-wins", flag: "first-wins", winner: "ZZ"},
		{name: "drop", flag: "drop", partial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, base...)
			if tc.flag != "" {
				args = append(args, "--node-property-conflict", tc.flag)
			}
			out, diag := invoke(t, bin, 0, args...)
			policy, state, resolved := tc.flag, "COMPLETE", "1"
			if policy == "" {
				policy = "last-wins"
			}
			if tc.partial {
				state, resolved = "PARTIAL", "0"
			}
			if !strings.Contains(out, "PUBLISHED_DURABLE "+state) || !strings.Contains(out, "node_property_conflict_policy="+policy+" resolved_node_property_conflicts="+resolved) {
				t.Fatal(out)
			}
			var event ports.Diagnostic
			if strings.Count(diag, "\n") != 1 || json.Unmarshal([]byte(diag), &event) != nil || strings.Contains(diag, "ZZ") || strings.Contains(diag, "AA") {
				t.Fatal(diag)
			}
			if tc.partial {
				if event.Code != "PROPERTY_CONFLICT" || event.Severity != ports.Rejection || event.Resolution != "" {
					t.Fatal(event)
				}
			} else {
				wantRecord := uint64(3)
				if tc.flag == "first-wins" {
					wantRecord = 2
				}
				if event.Code != "NODE_PROPERTY_CONFLICT_RESOLVED" || event.Severity != ports.Warning || event.Resolution != policy || event.Location.Source != "n.csv" || event.Location.Record != wantRecord {
					t.Fatal(event)
				}
			}
			out, diag = invoke(t, bin, 0, "territory", "--snapshot", path, "--node", "A", "--format", "json")
			result := decodeResult(t, out)
			if result.PartialSnapshot != tc.partial || (diag != "") != tc.partial || len(result.Nodes) != 1 {
				t.Fatal(out, diag)
			}
			properties := result.Nodes[0].Properties
			if tc.partial {
				if len(properties) != 0 {
					t.Fatal(properties)
				}
			} else {
				if len(properties) != 1 || properties[0].Key != "sigla" || properties[0].Type != "String" || string(properties[0].Value) != `"`+tc.winner+`"` {
					t.Fatal(properties)
				}
			}
		})
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "last", "LAST-WINS"} {
		out, diag := invoke(t, bin, 2, append(append([]string{}, base...), "--node-property-conflict="+invalid)...)
		if out != "" || !strings.Contains(diag, "node-property-conflict") {
			t.Fatal(out, diag)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatal("invalid policy changed the published snapshot", err)
		}
	}
}
