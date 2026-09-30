package graphdata

import (
	"context"
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

func TestCSRRejectsRepeatedMembershipWithoutSeenBitmap(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, acrossOwners := range []bool{false, true} {
			d := Empty()
			d.Strings = []string{"", "A", "B", "L", "e0", "e1", "e2"}
			d.NodeIDs.Heap = []uint32{1, 2}
			d.NodeLabels.Heap = []uint32{3, 3}
			d.NodeLabelOffsets.Heap = []uint64{0, 1, 2}
			d.NodePropOffsets.Heap = []uint64{0, 0, 0}
			d.EdgeIDs.Heap = []uint32{4, 5, 6}
			d.Sources.Heap = []uint32{0, 0, 1}
			d.Targets.Heap = []uint32{0, 0, 1}
			d.EdgeLabels.Heap = []uint32{3, 3, 3}
			d.EdgePropOffsets.Heap = []uint64{0, 0, 0, 0}
			BuildCSR(d)
			if err := BuildIndexes(context.Background(), d); err != nil {
				t.Fatal(err)
			}
			if err := Validate(d); err != nil {
				t.Fatal(err)
			}
			ids := d.ForwardEdges.Heap
			if reverse {
				ids = d.ReverseEdges.Heap
			}
			if acrossOwners {
				ids[2] = ids[0]
			} else {
				ids[1] = ids[0]
			}
			if Validate(d) == nil {
				t.Fatal("duplicate replaced missing edge", reverse, acrossOwners)
			}
		}
	}
}
