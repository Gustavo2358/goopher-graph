package graph

import (
	"errors"
	"math"
	"unicode/utf8"
)

type ValueKind uint8

const (
	BoolKind ValueKind = iota + 1
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

var ErrInvalidValue = errors.New("invalid value")

type Value struct {
	kind    ValueKind
	payload uint64
	text    string
}

func BoolValue(v bool) Value {
	var b uint64
	if v {
		b = 1
	}
	return Value{kind: BoolKind, payload: b}
}
func IntegerValue(k ValueKind, v int64) (Value, error) {
	if k < ByteKind || k > LongKind || (k == ByteKind && (v < math.MinInt8 || v > math.MaxInt8)) || (k == ShortKind && (v < math.MinInt16 || v > math.MaxInt16)) || (k == IntKind && (v < math.MinInt32 || v > math.MaxInt32)) {
		return Value{}, ErrInvalidValue
	}
	return Value{kind: k, payload: uint64(v)}, nil
}
func DecimalValue(k ValueKind, v float64) (Value, error) {
	switch k {
	case FloatKind:
		b := math.Float32bits(float32(v))
		if math.IsNaN(v) {
			b = 0x7fc00000
		}
		return Value{kind: k, payload: uint64(b)}, nil
	case DoubleKind:
		b := math.Float64bits(v)
		if math.IsNaN(v) {
			b = 0x7ff8000000000000
		}
		return Value{kind: k, payload: b}, nil
	}
	return Value{}, ErrInvalidValue
}
func TextValue(k ValueKind, v string) (Value, error) {
	if k < StringKind || k > DatetimeKind || !utf8.ValidString(v) {
		return Value{}, ErrInvalidValue
	}
	return Value{kind: k, text: v}, nil
}
func (v Value) Kind() ValueKind    { return v.kind }
func (v Value) Bool() (bool, bool) { return v.payload != 0, v.kind == BoolKind }
func (v Value) Int64() (int64, bool) {
	return int64(v.payload), v.kind >= ByteKind && v.kind <= LongKind
}
func (v Value) Float64() (float64, bool) {
	if v.kind == FloatKind {
		return float64(math.Float32frombits(uint32(v.payload))), true
	}
	return math.Float64frombits(v.payload), v.kind == DoubleKind
}
func (v Value) Text() (string, bool) { return v.text, v.kind >= StringKind && v.kind <= DatetimeKind }
func (v Value) Equal(w Value) bool   { return v == w }
