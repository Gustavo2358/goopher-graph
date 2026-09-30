//go:build wasip1 && wasm

package sdk

//go:wasmimport gophergraph_v1 call
func call(op uint32, a, b uint64, params string, output *byte, capacity uint32) uint64
