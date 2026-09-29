package snapshot

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestStructuredCorruption(t *testing.T) {
	base, e := os.ReadFile("../fixtures/snapshot_reference/typed.snapshot")
	if e != nil {
		t.Fatal(e)
	}
	offset := func(b []byte, section int) int { return int(le.Uint64(b[64+(section-1)*32+8:])) }
	cases := map[string]func([]byte){"magic": func(b []byte) { b[0] ^= 1 }, "flags": func(b []byte) { b[16] = 2 }, "count overflow": func(b []byte) { le.PutUint64(b[64+24:], ^uint64(0)) }, "stride": func(b []byte) { b[68] = 4 }, "offset": func(b []byte) { le.PutUint64(b[72:], 0) }, "file size": func(b []byte) { b[24] ^= 1 }, "padding": func(b []byte) {
		for section := 1; section <= 24; section++ {
			end := offset(b, section) + int(le.Uint64(b[64+(section-1)*32+16:]))
			if end%64 != 0 {
				b[end] = 1
				return
			}
		}
		t.Fatal("fixture has no padding")
	}, "string offsets": func(b []byte) { le.PutUint64(b[offset(b, 1):], 1) }, "utf8": func(b []byte) { b[offset(b, 2)] = 255 }, "property tag": func(b []byte) { b[offset(b, 7)+4] = 255 }, "property reserved": func(b []byte) { b[offset(b, 7)+5] = 1 }, "property payload": func(b []byte) { le.PutUint64(b[offset(b, 7)+8:], ^uint64(0)) }, "csr edge id": func(b []byte) { le.PutUint32(b[offset(b, 16):], 9) }, "csr neighbor": func(b []byte) { le.PutUint32(b[offset(b, 15):], 9) }, "csr offsets": func(b []byte) { le.PutUint64(b[offset(b, 14):], 1) }, "label membership": func(b []byte) { le.PutUint32(b[offset(b, 21):], 99) }, "label missing": func(b []byte) { le.PutUint64(b[offset(b, 20)+16:], 0) }, "index reserved": func(b []byte) { b[offset(b, 23)+2] = 1 }, "property posting": func(b []byte) { le.PutUint32(b[offset(b, 24):], 99) }, "key index": func(b []byte) { le.PutUint32(b[offset(b, 22):], 0) }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			b := append([]byte(nil), base...)
			mutate(b)
			owner := &backing{data: b}
			g, e := Open(context.Background(), source{b: owner})
			if g != nil || !errors.Is(e, ErrCorruptSnapshot) || owner.closes != 1 {
				t.Fatal(g, e, owner.closes)
			}
		})
	}
	for n := 0; n < len(base); n++ {
		owner := &backing{data: base[:n]}
		g, e := Open(context.Background(), source{b: owner})
		if g != nil || e == nil || owner.closes != 1 {
			t.Fatal("truncation", n, e)
		}
	}
}
