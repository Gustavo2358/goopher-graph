//go:build linux && amd64 && residencybench

package mmap

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// These helpers exist only in the explicit benchmark build. They keep Linux
// policy in the adapter and are absent from normal server/library artifacts.
// Caller owns a private synced benchmark inode/mapping, never a serving graph.
func BenchmarkDropFileCache(fd int) error {
	return unix.Fadvise(fd, 0, 0, unix.FADV_DONTNEED)
}

func BenchmarkLock(data []byte) error   { return unix.Mlock(data) }
func BenchmarkUnlock(data []byte) error { return unix.Munlock(data) }

func BenchmarkPrepare(data []byte, fd int, condition string) error {
	switch condition {
	case "cold":
		if err := unix.Madvise(data, unix.MADV_DONTNEED); err != nil {
			return err
		}
		return BenchmarkDropFileCache(fd)
	case "warm", "warm-cache", "locked":
		if err := unix.Madvise(data, unix.MADV_POPULATE_READ); err != nil {
			return err
		}
		if condition == "warm-cache" {
			return unix.Madvise(data, unix.MADV_DONTNEED)
		}
		if condition == "locked" {
			return BenchmarkLock(data)
		}
		return nil
	default:
		return fmt.Errorf("unknown benchmark condition %q", condition)
	}
}
