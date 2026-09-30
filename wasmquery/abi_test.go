package wasmquery

import (
	"context"
	"errors"
	"math"
	"testing"

	"gophergraph/graph"
	"gophergraph/wasmquery/internal/abi"
)

// A small independent binary encoder exercises hostile callers without the SDK.
// It emits one memory and a WASI _start calling a configurable host import.
func uleb(v uint64) []byte {
	var b []byte
	for {
		x := byte(v & 127)
		v >>= 7
		if v != 0 {
			x |= 128
		}
		b = append(b, x)
		if v == 0 {
			return b
		}
	}
}
func sleb(v int64) []byte {
	var b []byte
	for {
		x := byte(v & 127)
		v >>= 7
		done := (v == 0 && x&64 == 0) || (v == -1 && x&64 != 0)
		if !done {
			x |= 128
		}
		b = append(b, x)
		if done {
			return b
		}
	}
}
func wasmName(s string) []byte { return append(uleb(uint64(len(s))), s...) }
func tinyModule(module, name string, params []byte, code, data []byte) []byte {
	out := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	section := func(id byte, b []byte) {
		out = append(out, id)
		out = append(out, uleb(uint64(len(b)))...)
		out = append(out, b...)
	}
	types := append([]byte{2, 0x60, byte(len(params))}, params...)
	types = append(types, 1, 0x7e, 0x60, 0, 0)
	section(1, types)
	imports := append([]byte{1}, wasmName(module)...)
	imports = append(imports, wasmName(name)...)
	imports = append(imports, 0, 0)
	section(2, imports)
	section(3, []byte{1, 1})
	section(5, []byte{1, 0, 1})
	exports := append([]byte{2}, wasmName("_start")...)
	exports = append(exports, 0, 1)
	exports = append(exports, wasmName("memory")...)
	exports = append(exports, 2, 0)
	section(7, exports)
	body := append([]byte{0}, code...)
	body = append(body, 0x0b)
	payload := append([]byte{1}, uleb(uint64(len(body)))...)
	section(10, append(payload, body...))
	if data != nil {
		segment := []byte{1, 0, 0x41, 0, 0x0b}
		segment = append(segment, uleb(uint64(len(data)))...)
		section(11, append(segment, data...))
	}
	return out
}

var hostTypes = []byte{0x7f, 0x7e, 0x7e, 0x7f, 0x7f, 0x7f, 0x7f}

func callBytes(op uint32, a, b uint64, ptr, size, out, capacity uint32) []byte {
	var code []byte
	i32 := func(v uint32) { code = append(code, 0x41); code = append(code, sleb(int64(int32(v)))...) }
	i64 := func(v uint64) { code = append(code, 0x42); code = append(code, sleb(int64(v))...) }
	i32(op)
	i64(a)
	i64(b)
	i32(ptr)
	i32(size)
	i32(out)
	i32(capacity)
	return append(code, 0x10, 0, 0x1a)
}
func TestABIBoundaryAndImportRestrictions(t *testing.T) {
	r := newRuntime(t, Limits{})
	g := loadGraph(t, "../fixtures/01_topology", false)
	for _, tc := range []struct {
		name       string
		code, data []byte
		want       error
	}{
		{"out of bounds input", callBytes(abi.Nodes, 0, 0, 65530, 10, 0, 0), nil, ErrABI},
		{"overflow input", callBytes(abi.Nodes, 0, 0, math.MaxUint32, 10, 0, 0), nil, ErrABI},
		{"out of bounds output", callBytes(abi.Nodes, 0, 0, 0, 0, 65530, 10), nil, ErrABI},
		{"message cap", callBytes(abi.Nodes, 0, 0, 0, abi.MaxMessage+1, 0, 0), nil, ErrLimit},
		{"unknown field", callBytes(abi.Nodes, 0, 0, 0, 7, 0, 0), []byte(`{"x":1}`), ErrABI},
		{"trailing data", callBytes(abi.Nodes, 0, 0, 0, 5, 0, 0), []byte(`{} {}`), ErrABI},
		{"UTF8", callBytes(abi.Nodes, 0, 0, 0, 1, 0, 0), []byte{255}, ErrABI},
		{"unknown opcode", callBytes(999, 0, 0, 0, 0, 0, 0), nil, ErrABI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := r.Compile(context.Background(), tinyModule(abi.Module, "call", hostTypes, tc.code, tc.data))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = r.Execute(context.Background(), m, g, nil); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			assertClean(t, r)
		})
	}
	for _, pair := range [][2]string{{"env", "open"}, {"gophergraph_v2", "call"}, {abi.Module, "snapshot"}} {
		if _, err := r.Compile(context.Background(), tinyModule(pair[0], pair[1], hostTypes, nil, nil)); !errors.Is(err, ErrABI) {
			t.Fatal(err)
		}
	}
	if _, err := r.Compile(context.Background(), tinyModule(abi.Module, "call", nil, nil, nil)); !errors.Is(err, ErrABI) {
		t.Fatal("wrong signature accepted", err)
	}
}
func TestTypedValueWire(t *testing.T) {
	for _, v := range []abi.Value{
		{Kind: 1, Bits: 2}, {Kind: 2, Bits: 128}, {Kind: 3, Bits: 32768}, {Kind: 4, Bits: 1 << 31},
		{Kind: 6, Bits: 1 << 32}, {Kind: 8, Bits: 1}, {Kind: 0}, {Kind: 11}, {Kind: 1, Text: "x"},
	} {
		if _, err := decodeValue(v); !errors.Is(err, graph.ErrInvalidValue) {
			t.Fatal(v, err)
		}
	}
	for _, v := range []abi.Value{
		{Kind: 1, Bits: 1}, {Kind: 2, Bits: math.MaxUint64}, {Kind: 3, Bits: 1}, {Kind: 4, Bits: 2}, {Kind: 5, Bits: 1 << 63},
		{Kind: 6, Bits: 1 << 31}, {Kind: 7, Bits: 1 << 63}, {Kind: 6, Bits: 0x7fc00000}, {Kind: 7, Bits: 0x7ff8000000000000},
		{Kind: 8, Text: "☃"}, {Kind: 9, Text: "2026-09-30"}, {Kind: 10, Text: "2026-09-30T00:00:00Z"},
	} {
		g, err := decodeValue(v)
		if err != nil {
			t.Fatal(v, err)
		}
		if got := encodeValue(g); got != v {
			t.Fatal(got, v)
		}
	}
}

func addTable(wasm []byte, table []byte) []byte {
	// Insert before memory section, preserving WASM section order.
	for pos := 8; pos < len(wasm); {
		start := pos
		id := wasm[pos]
		pos++
		size, n, _ := readU32(wasm[pos:])
		pos += n
		if id == 5 {
			out := append([]byte(nil), wasm[:start]...)
			out = append(out, 4)
			out = append(out, uleb(uint64(len(table)))...)
			out = append(out, table...)
			return append(out, wasm[start:]...)
		}
		pos += int(size)
	}
	return nil
}
func TestFunctionTableLimit(t *testing.T) {
	r := newRuntime(t, Limits{TableElements: 16})
	g := loadGraph(t, "../fixtures/01_topology", false)
	// table.grow(100) must return -1. Trap if the cap was bypassed.
	code := []byte{0xd0, 0x70, 0x41, 0xe4, 0, 0xfc, 15, 0, 0x41, 0x7f, 0x47, 0x04, 0x40, 0x00, 0x0b}
	wasm := addTable(tinyModule(abi.Module, "call", hostTypes, code, nil), []byte{1, 0x70, 0, 1})
	m, err := r.Compile(context.Background(), wasm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Execute(context.Background(), m, g, nil); !errors.Is(err, ErrResult) {
		t.Fatalf("table limit not enforced: %v", err)
	}
	for _, table := range [][]byte{{1, 0x70, 0, 17}, {1, 0x70, 1, 1, 17}, {2, 0x70, 0, 1, 0x70, 0, 1}, {1, 0x6f, 0, 1}} {
		if _, err = r.Compile(context.Background(), addTable(tinyModule(abi.Module, "call", hostTypes, nil, nil), table)); err == nil {
			t.Fatal("table accepted", table)
		}
	}
}
func FuzzModuleAdmission(f *testing.F) {
	f.Add(tinyModule(abi.Module, "call", hostTypes, nil, nil))
	f.Add(addTable(tinyModule(abi.Module, "call", hostTypes, nil, nil), []byte{1, 0x70, 0, 1}))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		_, _ = limitTable(data, 65536)
	})
}
