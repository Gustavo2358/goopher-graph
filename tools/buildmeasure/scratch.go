package main

import (
	"context"
	"gophergraph/ingest/ports"
)

// Instrument the actual scratch port without changing the production builder.
type scratchStats struct {
	Read, Written, Current, Peak, Mapped, PeakMapped uint64
	Files, PeakFiles                                 int
}
type measuredScratch struct {
	ports.Scratch
	stats *scratchStats
}

func (s measuredScratch) New(ctx context.Context) (ports.Workspace, error) {
	w, err := s.Scratch.New(ctx)
	if err != nil {
		return nil, err
	}
	return &measuredWorkspace{Workspace: w, stats: s.stats}, nil
}

type measuredWorkspace struct {
	ports.Workspace
	stats *scratchStats
}

func (w *measuredWorkspace) Create() (ports.ScratchFile, error) {
	f, err := w.Workspace.Create()
	if err != nil {
		return nil, err
	}
	w.stats.Files++
	w.stats.PeakFiles = max(w.stats.PeakFiles, w.stats.Files)
	return &measuredFile{ScratchFile: f, stats: w.stats}, nil
}
func (w *measuredWorkspace) Remove(f ports.ScratchFile) error {
	file := f.(*measuredFile)
	w.stats.Current -= file.size
	w.stats.Files--
	return w.Workspace.Remove(file.ScratchFile)
}
func (w *measuredWorkspace) Map(ctx context.Context, f ports.ScratchFile) (ports.Mapping, error) {
	b, err := w.Workspace.Map(ctx, f.(*measuredFile).ScratchFile)
	if err != nil {
		return nil, err
	}
	w.stats.Mapped += uint64(len(b.Bytes()))
	w.stats.PeakMapped = max(w.stats.PeakMapped, w.stats.Mapped)
	return &measuredMapping{Mapping: b, stats: w.stats, size: uint64(len(b.Bytes()))}, nil
}
func (w *measuredWorkspace) Close() error {
	w.stats.Current = 0
	w.stats.Files = 0
	return w.Workspace.Close()
}

type measuredFile struct {
	ports.ScratchFile
	stats *scratchStats
	size  uint64
}

func (f *measuredFile) Write(b []byte) (int, error) {
	n, err := f.ScratchFile.Write(b)
	f.size += uint64(n)
	f.stats.Written += uint64(n)
	f.stats.Current += uint64(n)
	f.stats.Peak = max(f.stats.Peak, f.stats.Current)
	return n, err
}

func (f *measuredFile) ReadAt(b []byte, off int64) (int, error) {
	n, err := f.ScratchFile.ReadAt(b, off)
	f.stats.Read += uint64(n)
	return n, err
}

type measuredMapping struct {
	ports.Mapping
	stats *scratchStats
	size  uint64
}

func (m *measuredMapping) Close() error { m.stats.Mapped -= m.size; return m.Mapping.Close() }
