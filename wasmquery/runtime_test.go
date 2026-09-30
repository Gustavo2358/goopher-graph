package wasmquery

import (
	"context"
	"errors"
	"testing"
)

// A malformed module must never become an executable cache entry.
func TestInvalidModuleAndClosedRuntime(t *testing.T) {
	r, err := New(context.Background(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Compile(context.Background(), []byte("not wasm")); err == nil {
		t.Fatal("accepted malformed module")
	}
	if r.Stats().CachedModules != 0 {
		t.Fatal("cached failed compilation")
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Compile(context.Background(), nil); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
