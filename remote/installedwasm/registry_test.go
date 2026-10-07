package installedwasm

import (
	"context"
	"gophergraph/wasmquery"
	"testing"
)

func TestDefaultRegistryPreparedAndImmutable(t *testing.T) {
	r, err := wasmquery.New(context.Background(), wasmquery.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	reg, err := Default(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if stats := r.Stats(); stats.Compilations != 3 || stats.Executions != 0 || stats.ActiveExecutions != 0 {
		t.Fatal(stats)
	}
	list := reg.List()
	if len(list) != 3 || list[0].Name != "between" || len(reg.SHA256()) != 64 {
		t.Fatal(list)
	}
	list[0].Parameters[0] = "corrupt"
	list[0].Name = "other"
	d, m, ok := reg.Lookup("between")
	if !ok || d.Parameters[0] != "from" || m.SHA256() != d.SHA256 {
		t.Fatal(d)
	}
	d.Parameters[0] = "bad"
	if reg.List()[0].Parameters[0] != "from" {
		t.Fatal("mutable alias")
	}
	if _, _, ok = reg.Lookup("missing"); ok {
		t.Fatal("lookup")
	}
	b, _ := assets.ReadFile("assets/between.wasm")
	for _, installations := range [][]Installation{
		{{Name: "bad name", MinArgs: 0, MaxArgs: 0, Wasm: b}},
		{{Name: "bad", Wasm: []byte("invalid")}},
		{{Name: "same", Wasm: b}, {Name: "same", Wasm: b}},
	} {
		if _, err = Load(context.Background(), r, installations); err == nil {
			t.Fatal("invalid installation accepted")
		}
	}
}
