package graphdata

import (
	"encoding/binary"
	"testing"
)

func TestColumnBackends(t *testing.T) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, 42)
	if (U32{Bytes: b[:4]}).At(0) != 42 || (U64{Bytes: b[:8]}).At(0) != 42 {
		t.Fatal("LE")
	}
	b[4] = 5
	binary.LittleEndian.PutUint64(b[8:], 99)
	p := (Properties{Bytes: b}).At(0)
	if p.Key != 42 || p.Kind != 5 || p.Payload != 99 {
		t.Fatal(p)
	}
}
func TestInvalidColumns(t *testing.T) {
	for _, mutate := range []func(*Data){func(d *Data) { d.Strings = []string{"bad"} }, func(d *Data) { d.NodeIDs.Heap = []uint32{1} }, func(d *Data) { d.ForwardOffsets.Heap = []uint64{1} }, func(d *Data) { d.EdgeIDs.Heap = []uint32{0} }} {
		d := Empty()
		mutate(d)
		if Validate(d) == nil {
			t.Fatal("accepted malformed columns")
		}
	}
}
