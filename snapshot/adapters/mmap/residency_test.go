package mmap

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func residentFixture(t *testing.T, mode Mode) (*Resident, []byte) {
	t.Helper()
	data := make([]byte, 4<<20|123)
	for i := range data {
		data[i] = byte(i)
	}
	p := filepath.Join(t.TempDir(), "mapping")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewResident(p, mode)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return r, data
}
func TestResidencyContracts(t *testing.T) {
	for _, mode := range []Mode{Locked, Warm, Lazy} {
		t.Run(string(mode), func(t *testing.T) {
			r, data := residentFixture(t, mode)
			report, err := r.Prepare(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if report.SnapshotID != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) || report.WarmCompleted != (mode != Lazy) || report.Locked != (mode == Locked) {
				t.Fatalf("%+v", report)
			}
			page := uint64(os.Getpagesize())
			if mode == Locked && report.LockedBytes != (uint64(len(data))+page-1)/page*page {
				t.Fatal(report)
			}
			if _, err := r.Inspect(); err != nil {
				t.Log("optional inspection unavailable:", err)
			}
			if _, err = r.Acquire(context.Background()); err == nil {
				t.Fatal("multiple backing acquisition")
			}
		})
	}
}
func TestPrefaultFailuresNeverDowngrade(t *testing.T) {
	r, _ := residentFixture(t, Locked)
	r.populate = func([]byte, int) error { return unix.EIO }
	if _, err := r.Prepare(context.Background()); !errors.Is(err, unix.EIO) {
		t.Fatal(err)
	}
	r.populate = func([]byte, int) error { return unix.EINVAL } // explicitly supported fallback
	r.lock = func([]byte) error { return unix.ENOMEM }
	report, err := r.Prepare(context.Background())
	if !errors.Is(err, unix.ENOMEM) || report.Locked || r.prepared {
		t.Fatal("silent downgrade", report, err)
	}
	c, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Prepare(c); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := NewResident("x", Mode("other")); err == nil {
		t.Fatal("invalid mode")
	}
}

func TestPrepareCancellationAfterLockStillCleansUp(t *testing.T) {
	r, _ := residentFixture(t, Locked)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.lock = func(data []byte) error { err := unix.Mlock(data); cancel(); return err }
	if _, err := r.Prepare(ctx); !errors.Is(err, context.Canceled) || r.prepared || !r.back.locked {
		t.Fatal("late cancellation ignored", err)
	}
}
func TestLockedMemlockLimit(t *testing.T) {
	if os.Getenv("GOPHERGRAPH_MEMLOCK_CHILD") == "1" {
		if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
			t.Fatal(err)
		}
		r, _ := residentFixture(t, Locked)
		report, err := r.Prepare(context.Background())
		if err == nil || report.Locked || r.prepared {
			t.Fatal("mlock bypassed zero budget")
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockedMemlockLimit$")
	cmd.Env = append(os.Environ(), "GOPHERGRAPH_MEMLOCK_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
}
