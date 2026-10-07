package remote

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestAdmissionNoQueueAndDrain(t *testing.T) {
	a, err := NewAdmission(2)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := a.Try()
	second, _ := a.Try()
	if _, err = a.Try(); !errors.Is(err, ErrSaturated) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = a.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	a.Drain()
	if _, err = a.Try(); !errors.Is(err, ErrDraining) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(first)
	}
	wg.Wait()
	second()
	if err = a.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := a.Stats()
	if s.Active != 0 || s.Admitted != 2 || s.Rejected != 1 || !s.Draining {
		t.Fatal(s)
	}
}
