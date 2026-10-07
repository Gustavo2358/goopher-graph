// Package remote adapts the existing graph/query/WASM/GGPB capabilities to gRPC.
package remote

import (
	"context"
	"errors"
	"sync"
)

var ErrSaturated = errors.New("query admission saturated")
var ErrDraining = errors.New("server draining")

// Admission bounds the entire query lifecycle, including a blocked send.
// Try never queues. Drain seals Add before Wait, preventing WaitGroup misuse.
type Admission struct {
	mu                 sync.Mutex
	capacity, active   int
	draining           bool
	idle               chan struct{}
	admitted, rejected uint64
}
type AdmissionStats struct {
	Capacity, Active   int
	Draining           bool
	Admitted, Rejected uint64
}

func NewAdmission(capacity int) (*Admission, error) {
	if capacity < 1 {
		return nil, errors.New("query capacity must be positive")
	}
	idle := make(chan struct{})
	close(idle)
	return &Admission{capacity: capacity, idle: idle}, nil
}
func (a *Admission) Try() (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.draining {
		return nil, ErrDraining
	}
	if a.active == a.capacity {
		a.rejected++
		return nil, ErrSaturated
	}
	if a.active == 0 {
		a.idle = make(chan struct{})
	}
	a.active++
	a.admitted++
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.active--
			if a.active == 0 {
				close(a.idle)
			}
		})
	}, nil
}
func (a *Admission) Drain() { a.mu.Lock(); a.draining = true; a.mu.Unlock() }
func (a *Admission) Wait(ctx context.Context) error {
	a.mu.Lock()
	idle := a.idle
	a.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (a *Admission) Stats() AdmissionStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return AdmissionStats{a.capacity, a.active, a.draining, a.admitted, a.rejected}
}
