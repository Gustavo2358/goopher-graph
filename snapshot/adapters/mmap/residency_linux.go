//go:build linux && amd64

package mmap

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"gophergraph/snapshot/ports"
)

type Mode string

const (
	Locked Mode = "locked"
	Warm   Mode = "warm"
	Lazy   Mode = "lazy"
)

// Residency is an established startup contract, not a promise that warm pages
// cannot subsequently be reclaimed. Inspection is always optional telemetry.
type Residency struct {
	Mode                  Mode
	Size                  uint64
	WarmCompleted, Locked bool
	LockedBytes           uint64
	PrepareTime           time.Duration
	SnapshotID            string
}

// Resident owns preparation of exactly one backing. After successful Open,
// Graph owns its Close. Prepare/Inspect must not race with Close. No hot-swap.
type Resident struct {
	path     string
	mode     Mode
	back     *backing
	report   Residency
	prepared bool
	populate func([]byte, int) error
	lock     func([]byte) error
}

func NewResident(path string, mode Mode) (*Resident, error) {
	if mode == "" {
		mode = Locked
	}
	if mode != Locked && mode != Warm && mode != Lazy {
		return nil, fmt.Errorf("unknown residency mode %q", mode)
	}
	return &Resident{path: path, mode: mode, populate: unix.Madvise, lock: unix.Mlock}, nil
}
func (r *Resident) Acquire(ctx context.Context) (ports.Backing, error) {
	if r.back != nil {
		return nil, errors.New("resident source already acquired")
	}
	b, err := New(r.path).Acquire(ctx)
	if err != nil {
		return nil, err
	}
	r.back = b.(*backing)
	return b, nil
}

// Prepare visits all bytes once for SHA identity, fused with chunked prefault.
// MADV_POPULATE_READ reports paging errors; older kernels fall back only on
// unsupported advice. Hashing reads every byte, so the fallback touches every
// mapped page as well. Lock applies to this mapping only (never mlockall).
// A failed operation never downgrades the requested mode.
func (r *Resident) Prepare(ctx context.Context) (Residency, error) {
	if r.prepared {
		return r.report, nil
	}
	if r.back == nil || r.back.closed {
		return Residency{}, errors.New("resident backing unavailable")
	}
	started := time.Now()
	data := r.back.data
	report := Residency{Mode: r.mode, Size: uint64(len(data))}
	h := sha256.New()
	unsupported := false
	for offset := 0; offset < len(data); offset += 4 << 20 {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		chunk := data[offset:min(offset+4<<20, len(data))]
		if r.mode != Lazy && !unsupported {
			err := r.populate(chunk, unix.MADV_POPULATE_READ)
			if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
				unsupported = true
			} else if err != nil {
				return report, fmt.Errorf("prefault snapshot: %w", err)
			}
		}
		_, _ = h.Write(chunk)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.SnapshotID = fmt.Sprintf("sha256:%x", h.Sum(nil))
	report.WarmCompleted = r.mode != Lazy
	if r.mode == Locked {
		if err := r.lock(data); err != nil {
			var lim unix.Rlimit
			_ = unix.Getrlimit(unix.RLIMIT_MEMLOCK, &lim)
			return report, fmt.Errorf("mlock %d bytes failed (RLIMIT_MEMLOCK soft=%d hard=%d; check CAP_IPC_LOCK, seccomp and cgroup memory): %w", len(data), lim.Cur, lim.Max, err)
		}
		r.back.locked = true
		report.Locked = true
		page := uint64(os.Getpagesize())
		report.LockedBytes = (uint64(len(data)) + page - 1) / page * page
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.PrepareTime = time.Since(started)
	r.report = report
	r.prepared = true
	return report, nil
}

type Observation struct {
	At                                 time.Time
	RSSBytes, PSSBytes, LockedPSSBytes uint64
}

// Inspect is diagnostic only. smaps Locked is proportional (PSS), not the
// mlock byte count. Restricted /proc cannot change readiness or the contract.
func (r *Resident) Inspect() (Observation, error) {
	if r.back == nil || r.back.closed {
		return Observation{}, errors.New("resident backing unavailable")
	}
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		return Observation{}, err
	}
	defer f.Close()
	start, err := strconv.ParseUint(strings.TrimPrefix(fmt.Sprintf("%p", &r.back.data[0]), "0x"), 16, 64)
	if err != nil {
		return Observation{}, err
	}
	return inspectMapping(f, start, start+uint64(len(r.back.data)))
}
func inspectMapping(f *os.File, start, end uint64) (Observation, error) {
	o := Observation{At: time.Now()}
	scanner := bufio.NewScanner(f)
	match := false
	found := false
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if bounds := strings.Split(fields[0], "-"); len(bounds) == 2 {
			a, e1 := strconv.ParseUint(bounds[0], 16, 64)
			b, e2 := strconv.ParseUint(bounds[1], 16, 64)
			match = e1 == nil && e2 == nil && a < end && b > start
			found = found || match
			continue
		}
		if !match {
			continue
		}
		v, e := strconv.ParseUint(fields[1], 10, 64)
		if e != nil {
			continue
		}
		switch fields[0] {
		case "Rss:":
			o.RSSBytes += v * 1024
		case "Pss:":
			o.PSSBytes += v * 1024
		case "Locked:":
			o.LockedPSSBytes += v * 1024
		}
	}
	if err := scanner.Err(); err != nil {
		return o, err
	}
	if !found {
		return o, errors.New("snapshot VMA unavailable")
	}
	return o, nil
}
