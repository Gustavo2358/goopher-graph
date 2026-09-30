// Package abi defines the versioned, bounded control messages shared by host and SDK.
package abi

const Module = "gophergraph_v1"
const Failure = ^uint64(0)
const MaxMessage = 64 << 10

const (
	Nodes uint32 = iota + 1
	NodesLabel
	Out
	In
	Both
	OutE
	InE
	BothE
	Sources
	Targets
	Label
	Property
	Union
	Intersection
	Difference
	Reachable
	Induced
	Subgraph
	SubNodes
	SubEdges
	Count
	Empty
	ReadNodeProperty
	ReadEdgeProperty
	Release
	Return
)

// Value carries exact typed bits, including int64, signed zero and nonfinite floats.
// Kinds match the ten public graph value kinds. Text kinds have Bits == 0.
type Value struct {
	Kind uint8  `json:"kind"`
	Bits uint64 `json:"bits,omitempty"`
	Text string `json:"text,omitempty"`
}
type Params struct {
	IDs       []string `json:"ids,omitempty"`
	Label     string   `json:"label,omitempty"`
	Key       string   `json:"key,omitempty"`
	Value     Value    `json:"value,omitempty"`
	Direction uint32   `json:"direction,omitempty"`
	Labels    []string `json:"labels"`
	ID        string   `json:"id,omitempty"`
	Index     uint32   `json:"index,omitempty"`
}
type PropertyResult struct {
	Found bool  `json:"found"`
	Value Value `json:"value"`
}
