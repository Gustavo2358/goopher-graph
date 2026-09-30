//go:build !wasip1 || !wasm

package sdk

import "gophergraph/wasmquery/internal/abi"

// The authoring API can be inspected on native Go, but requires a WASM host to run.
func call(uint32, uint64, uint64, string, *byte, uint32) uint64 { return abi.Failure }
