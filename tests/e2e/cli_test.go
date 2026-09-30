package e2e

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func binary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gophergraph")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/gophergraph")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build %v %s", e, out)
	}
	return bin
}
func invoke(t *testing.T, bin string, want int, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var out, diag bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diag
	e := cmd.Run()
	code := 0
	if e != nil {
		if v, ok := e.(*exec.ExitError); ok {
			code = v.ExitCode()
		} else {
			t.Fatal(e)
		}
	}
	if code != want {
		t.Fatalf("%v exit=%d want=%d stdout=%s stderr=%s", args, code, want, out.String(), diag.String())
	}
	return out.String(), diag.String()
}
func TestCLICompletePartialAndQueries(t *testing.T) {
	bin := binary(t)
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "g.snapshot")
	out, diag := invoke(t, bin, 0, "build", "--nodes", "../../fixtures/01_topology/nodes", "--edges", "../../fixtures/01_topology/edges", "--output", snapshot, "--index-property", "sigla")
	if !strings.Contains(out, "COMPLETE") || strings.Contains(diag, "PARTIAL") {
		t.Fatal(out, diag)
	}
	for _, command := range []string{"territory", "anti-territory", "between"} {
		args := []string{command, "--snapshot", snapshot}
		if command == "between" {
			args = append(args, "--from", "A", "--to", "F")
		} else {
			args = append(args, "--node", "A")
		}
		out, _ = invoke(t, bin, 0, args...)
		want := map[string]string{"territory": "id\nB\nC\nD\nF\n", "anti-territory": "id\n", "between": "id\nA\nB\nC\nD\nF\n"}[command]
		if out != want {
			t.Fatalf("%s: %q want %q", command, out, want)
		}
		args = append(args, "--format", "dot")
		out, _ = invoke(t, bin, 0, args...)
		if !strings.HasPrefix(out, "digraph G {\n") || strings.Contains(out, "strict") {
			t.Fatal(out)
		}
	}
	out, _ = invoke(t, bin, 0, "territory", "--snapshot", snapshot, "--node", "A", "--edge-label", "unknown")
	if out != "id\n" {
		t.Fatal(out)
	}
	invoke(t, bin, 3, "territory", "--snapshot", snapshot, "--node", "missing")
	invoke(t, bin, 2, "territory", "--snapshot", snapshot)
	invoke(t, bin, 2, "territory", "--snapshot", snapshot, "--node", "A", "--output", snapshot)
	invoke(t, bin, 2, "build", "--nodes", "../../fixtures/01_topology/nodes", "--edges", "../../fixtures/01_topology/nodes", "--output", snapshot)
	invoke(t, bin, 2, "build", "--nodes", "../../fixtures/01_topology/nodes", "--edges", "../../fixtures/01_topology/edges", "--output", snapshot, "--max-record-bytes", "0")
	out, _ = invoke(t, bin, 0, "build", "--nodes", "../../fixtures/02_resilient/nodes", "--edges", "../../fixtures/02_resilient/edges", "--output", snapshot)
	if !strings.Contains(out, "PARTIAL") {
		t.Fatal(out)
	}
	out, diag = invoke(t, bin, 0, "territory", "--snapshot", snapshot, "--node", "A")
	if !strings.Contains(diag, "PARTIAL") || !strings.HasPrefix(out, "id\n") {
		t.Fatal(out, diag)
	}
}
func TestEmptyAndMultilineIDs(t *testing.T) {
	bin := binary(t)
	dir := t.TempDir()
	nodes, edges := filepath.Join(dir, "nodes"), filepath.Join(dir, "edges")
	for _, p := range []string{nodes, edges} {
		if e := os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(nodes, "x"), []byte("~id\n\"\"\n\"line1\nline2\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(edges, "x"), []byte("~id,~from,~to\ne,\"\",\"line1\nline2\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	snapshot := filepath.Join(dir, "snapshot")
	invoke(t, bin, 0, "build", "--nodes", nodes, "--edges", edges, "--output", snapshot)
	out, _ := invoke(t, bin, 0, "territory", "--snapshot", snapshot, "--node=", "--include-origin")
	if !strings.Contains(out, "\n\"\"\n") {
		t.Fatal(out)
	}
	records, e := csv.NewReader(strings.NewReader(out)).ReadAll()
	if e != nil || len(records) != 3 || records[1][0] != "" || records[2][0] != "line1\nline2" {
		t.Fatal(records, e)
	}
	invoke(t, bin, 2, "build", "--nodes", nodes, "--edges", edges, "--output", filepath.Join(nodes, "snapshot"))
}
func TestExternalConsumer(t *testing.T) {
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	mod := fmt.Sprintf("module consumer\n\ngo 1.26\n\nrequire gophergraph v0.0.0\nreplace gophergraph => %s\n", root)
	if e = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); e != nil {
		t.Fatal(e)
	}
	source := `package main
import("context";"fmt";"os"; shared "gophergraph/examples/shared_targets";"gophergraph/snapshot";"gophergraph/snapshot/adapters/mmap")
func main(){g,e:=snapshot.Open(context.Background(),mmap.New(os.Args[1]));if e!=nil{panic(e)};defer g.Close();s,e:=shared.Execute(context.Background(),g,"A","B");if e!=nil{panic(e)};fmt.Println(s.Count())}
`
	if e = os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	sum, e := os.ReadFile(filepath.Join(root, "go.sum"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("go", "run", "-mod=mod", ".", filepath.Join(root, "fixtures/snapshot_reference/topology.snapshot"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0")
	out, e := cmd.CombinedOutput()
	if e != nil || strings.TrimSpace(string(out)) != "3" {
		t.Fatalf("consumer %v: %s", e, out)
	}
}

func TestCLIBoundedBuildAndScratchFailure(t *testing.T) {
	bin := binary(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "graph")
	nodes, edges := "../../fixtures/01_topology/nodes", "../../fixtures/01_topology/edges"
	base := []string{"build", "--nodes", nodes, "--edges", edges, "--output", path}
	invoke(t, bin, 0, append(base, "--memory-budget", "1048576", "--temp-dir", dir)...)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, bin, 2, append(base, "--memory-budget", "1024")...)
	invoke(t, bin, 2, append(base, "--temp-dir", nodes)...)
	// A regular file cannot be a scratch parent; publication must preserve the
	// previous snapshot and leave it queryable after this operational failure.
	invoke(t, bin, 1, append(base, "--temp-dir", path)...)
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("previous snapshot changed", err)
	}
	invoke(t, bin, 0, "territory", "--snapshot", path, "--node", "A")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("scratch leak", entries, err)
	}
}
