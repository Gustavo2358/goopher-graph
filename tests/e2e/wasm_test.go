package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestWasmCLIAndExternalSDK(t *testing.T) {
	bin := binary(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// An independent Go module can author a guest using only the public SDK.
	mod := "module externalquery\n\ngo 1.26\n\nrequire gophergraph v0.0.0\nreplace gophergraph => " + strconv.Quote(root) + "\n"
	if err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../examples/wasm/between/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "main.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(dir, "external.wasm")
	// This temporary standalone module has no Git repository. Build its real
	// WASM without asking Git to stamp metadata from a surrounding checkout.
	build := exec.Command("go", "build", "-buildvcs=false", "-o", module, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("external Go SDK: %v %s", err, out)
	}
	snapshot := filepath.Join(dir, "graph.snapshot")
	for _, fixture := range []string{"01_topology", "02_resilient"} {
		invoke(t, bin, 0, "build", "--nodes", "../../fixtures/"+fixture+"/nodes", "--edges", "../../fixtures/"+fixture+"/edges", "--output", snapshot)
		args := []string{"wasm", "--snapshot", snapshot, "--module", module, "--arg", "A", "--arg", "A"}
		actual, diag := invoke(t, bin, 0, args...)
		native, _ := invoke(t, bin, 0, "between", "--snapshot", snapshot, "--from", "A", "--to", "A", "--format", "json")
		var got, want map[string]json.RawMessage
		if err = json.Unmarshal([]byte(actual), &got); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal([]byte(native), &want); err != nil {
			t.Fatal(err)
		}
		delete(got, "query")
		delete(want, "query")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("WASM=%s native=%s", actual, native)
		}
		if strings.Contains(diag, "PARTIAL") != (fixture == "02_resilient") {
			t.Fatal(diag)
		}
		again, _ := invoke(t, bin, 0, args...)
		if actual != again {
			t.Fatal("nondeterministic output")
		}
		wire := filepath.Join(dir, "wasm.ggpb")
		invoke(t, bin, 0, append(args, "--format", "ggpb", "--output", wire)...)
		decoded, _ := invoke(t, bin, 0, "decode", "--input", wire, "--format", "json")
		if decoded != actual {
			t.Fatal("WASM GGPB parity", decoded, actual)
		}
		invoke(t, bin, 2, append(args, "--format", "invalid")...)
		output := filepath.Join(dir, "result.json")
		invoke(t, bin, 0, append(args, "--output", output)...)
		data, err := os.ReadFile(output)
		if err != nil || string(data) != actual {
			t.Fatal(err, string(data))
		}
		invoke(t, bin, 2, append(args, "--output", snapshot)...)
		invoke(t, bin, 2, append(args, "--output", module)...)
	}
	invoke(t, bin, 2, "wasm", "--snapshot", snapshot)
	invoke(t, bin, 2, "wasm", "--snapshot", snapshot, "--module", module, "--timeout", "0")
	invoke(t, bin, 1, "wasm", "--snapshot", snapshot, "--module", module)
	invoke(t, bin, 3, "wasm", "--snapshot", snapshot, "--module", module, "--arg", "missing", "--arg", "A")
	bad := filepath.Join(dir, "bad.wasm")
	if err = os.WriteFile(bad, []byte("bad module"), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, bin, 1, "wasm", "--snapshot", snapshot, "--module", bad)
}
