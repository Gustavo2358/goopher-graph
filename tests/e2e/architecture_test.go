package e2e

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCapabilityImports(t *testing.T) {
	root := "../.."
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			// .measure holds ignored copies used by the repository qualification tools.
			if d.Name() == ".git" || d.Name() == "bin" || d.Name() == ".measure" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		f, e := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if e != nil {
			return e
		}
		for _, i := range f.Imports {
			imp, e := strconv.Unquote(i.Path.Value)
			if e != nil {
				return e
			}
			if imp == "unsafe" {
				t.Errorf("own unsafe: %s", rel)
			}
			if imp == "golang.org/x/sys/unix" && !strings.HasPrefix(rel, "snapshot/adapters/mmap/") {
				t.Errorf("unix outside mmap: %s", rel)
			}
			if strings.HasPrefix(rel, "graph/") || strings.HasPrefix(rel, "internal/graphdata/") || strings.HasPrefix(rel, "query/") {
				if imp == "os" || imp == "io" || strings.Contains(imp, "/adapters/") || strings.HasPrefix(imp, "net") || strings.HasPrefix(imp, "path") || strings.Contains(imp, "ingest") || strings.Contains(imp, "snapshot") || strings.HasPrefix(imp, "gophergraph/ggpb") || strings.HasPrefix(imp, "google.golang.org/protobuf") || imp == "gophergraph/graphjson" || imp == "gophergraph/dot" {
					t.Errorf("core dependency %s imports %s", rel, imp)
				}
			}
			if strings.HasPrefix(rel, "ingest/") && !strings.Contains(rel, "/adapters/") && strings.Contains(imp, "/adapters/") {
				t.Errorf("ingest imports adapter: %s", imp)
			}
			if strings.HasPrefix(rel, "snapshot/") && !strings.Contains(rel, "/adapters/") && strings.Contains(imp, "/adapters/") {
				t.Errorf("snapshot imports adapter: %s", imp)
			}
			if strings.HasPrefix(rel, "graphjson/") {
				if imp == "os" || strings.HasPrefix(imp, "net") || strings.HasPrefix(imp, "path") || (strings.HasPrefix(imp, "gophergraph/") && imp != "gophergraph/graph" && imp != "gophergraph/query") {
					t.Errorf("JSON export dependency %s imports %s", rel, imp)
				}
			}
			if strings.HasPrefix(rel, "ggpb/") {
				if imp == "os" || strings.HasPrefix(imp, "net") || strings.HasPrefix(imp, "path") || strings.Contains(imp, "/adapters/") || strings.HasPrefix(imp, "gophergraph/snapshot") || strings.HasPrefix(imp, "gophergraph/ingest") || strings.Contains(imp, "internal/graphdata") {
					t.Errorf("GGPB imports storage/core internals: %s %s", rel, imp)
				}
			}
			if strings.HasPrefix(imp, "google.golang.org/protobuf") && !strings.HasPrefix(rel, "ggpb/") {
				t.Errorf("Protobuf outside result adapter: %s", rel)
			}
			if strings.HasPrefix(rel, "wasmquery/") && !strings.Contains(rel, "/testdata/") {
				if imp == "os" || strings.HasPrefix(imp, "net") || strings.HasPrefix(imp, "path") || strings.HasPrefix(imp, "gophergraph/snapshot") || strings.HasPrefix(imp, "gophergraph/ingest") || strings.Contains(imp, "internal/graphdata") {
					t.Errorf("WASM capability imports storage/transport: %s %s", rel, imp)
				}
				if strings.HasPrefix(rel, "wasmquery/sdk/") && (imp == "gophergraph/graph" || imp == "gophergraph/query" || strings.HasPrefix(imp, "github.com/")) {
					t.Errorf("SDK imports host implementation: %s", imp)
				}
			}
			if strings.HasPrefix(imp, "github.com/tetratelabs/wazero") && !strings.HasPrefix(rel, "wasmquery/") {
				t.Errorf("WASM runtime outside capability: %s", rel)
			}
			if strings.HasPrefix(rel, "cmd/") && strings.Contains(imp, "internal/") {
				t.Errorf("CLI imports internals: %s", imp)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
