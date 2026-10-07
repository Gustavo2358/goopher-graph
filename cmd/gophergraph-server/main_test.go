package main

import (
	"context"
	"testing"
)

func TestStartupRejectsInvalidInputs(t *testing.T) {
	for _, args := range [][]string{nil, {"--snapshot", "missing"}, {"--snapshot", "x", "--residency", "bad"}, {"--snapshot", "x", "--shutdown-grace=-1s"}, {"--snapshot", "x", "--wasm-memory-pages=65537"}} {
		if err := run(context.Background(), args); err == nil {
			t.Fatal(args)
		}
	}
}
func TestCapacityUsesCPUAndMemory(t *testing.T) {
	c, envelope, err := queryCapacity(0, 512<<20, 100000, 500000, 1024, 64<<20)
	if err != nil || c < 1 || uint64(c)*envelope > 512<<20 {
		t.Fatal(c, envelope, err)
	}
	if _, _, err = queryCapacity(0, 1, 100, 500, 1024, 64<<20); err == nil {
		t.Fatal("impossible budget")
	}
	if _, _, err = queryCapacity(1000, 512<<20, 100, 500, 1024, 64<<20); err == nil {
		t.Fatal("unbounded override")
	}
}
