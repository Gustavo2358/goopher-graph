package ingest

import (
	"math"
	"testing"
)

func TestPropertySequenceOverflow(t *testing.T) {
	b := builder{sequence: math.MaxUint64 - 1}
	if sequence, err := b.nextPropertySequence(); err != nil || sequence != math.MaxUint64 {
		t.Fatal(sequence, err)
	}
	if sequence, err := b.nextPropertySequence(); err == nil || sequence != 0 || b.sequence != math.MaxUint64 {
		t.Fatal("sequence wrapped instead of failing operationally", sequence, err, b.sequence)
	}
}
