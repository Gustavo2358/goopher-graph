package sdk

import (
	"gophergraph/wasmquery/internal/abi"
	"math"
)

// Value exposes its type tag and exact payload for point reads. Prefer the typed
// constructors for filters. Invalid payloads are rejected by the host.
type Value = abi.Value

const (
	BoolKind uint8 = iota + 1
	ByteKind
	ShortKind
	IntKind
	LongKind
	FloatKind
	DoubleKind
	StringKind
	DateKind
	DatetimeKind
)

func Bool(v bool) Value {
	b := uint64(0)
	if v {
		b = 1
	}
	return Value{Kind: BoolKind, Bits: b}
}
func Byte(v int8) Value       { return Value{Kind: ByteKind, Bits: uint64(int64(v))} }
func Short(v int16) Value     { return Value{Kind: ShortKind, Bits: uint64(int64(v))} }
func Int(v int32) Value       { return Value{Kind: IntKind, Bits: uint64(int64(v))} }
func Long(v int64) Value      { return Value{Kind: LongKind, Bits: uint64(v)} }
func Float(v float32) Value   { return Value{Kind: FloatKind, Bits: uint64(math.Float32bits(v))} }
func Double(v float64) Value  { return Value{Kind: DoubleKind, Bits: math.Float64bits(v)} }
func String(v string) Value   { return Value{Kind: StringKind, Text: v} }
func Date(v string) Value     { return Value{Kind: DateKind, Text: v} }
func Datetime(v string) Value { return Value{Kind: DatetimeKind, Text: v} }
