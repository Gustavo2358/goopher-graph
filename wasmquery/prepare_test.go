package wasmquery

import (
	"context"
	"errors"
	"testing"
)

func TestPrepareInstalledModule(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "../examples/wasm/between")
	if err := r.Prepare(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if s := r.Stats(); s.Executions != 0 || s.ActiveExecutions != 0 || s.ActiveHandles != 0 {
		t.Fatal(s)
	}
	if err := r.Prepare(context.Background(), nil); !errors.Is(err, ErrABI) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Prepare(ctx, m); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPrepareRejectsUnresolvedWASIImport(t *testing.T) {
	r := newRuntime(t, Limits{})
	for _, name := range []string{"does_not_exist", "args_get"} {
		// Compile accepts the WASI namespace; Prepare must resolve both name
		// and signature against the real host before registry publication.
		m, err := r.Compile(context.Background(), tinyModule("wasi_snapshot_preview1", name, nil, nil, nil))
		if err != nil {
			t.Fatal(err)
		}
		if err = r.Prepare(context.Background(), m); !errors.Is(err, ErrABI) {
			t.Fatal(name, err)
		}
	}
}
