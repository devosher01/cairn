package engine

import (
	"sync/atomic"

	"github.com/devosher01/cairn/internal/manifest"
)

const NumLevels = manifest.NumLevels

type counters struct {
	puts                   atomic.Uint64
	deletes                atomic.Uint64
	gets                   atomic.Uint64
	walBytesWritten        atomic.Uint64
	flushes                atomic.Uint64
	flushBytes             atomic.Uint64
	compactions            atomic.Uint64
	compactionBytesRead    atomic.Uint64
	compactionBytesWritten atomic.Uint64
	writeStalls            atomic.Uint64
}

type LevelMetrics struct {
	Tables int
	Bytes  int64
}

type Metrics struct {
	Puts                   uint64
	Deletes                uint64
	Gets                   uint64
	WALBytesWritten        uint64
	Flushes                uint64
	FlushBytes             uint64
	Compactions            uint64
	CompactionBytesRead    uint64
	CompactionBytesWritten uint64
	WriteStalls            uint64
	Levels                 [manifest.NumLevels]LevelMetrics
}

func (db *DB) Metrics() Metrics {
	out := Metrics{
		Puts:                   db.counters.puts.Load(),
		Deletes:                db.counters.deletes.Load(),
		Gets:                   db.counters.gets.Load(),
		WALBytesWritten:        db.counters.walBytesWritten.Load(),
		Flushes:                db.counters.flushes.Load(),
		FlushBytes:             db.counters.flushBytes.Load(),
		Compactions:            db.counters.compactions.Load(),
		CompactionBytesRead:    db.counters.compactionBytesRead.Load(),
		CompactionBytesWritten: db.counters.compactionBytesWritten.Load(),
		WriteStalls:            db.counters.writeStalls.Load(),
	}
	db.mu.Lock()
	if db.current != nil {
		for l, level := range db.current.levels {
			for _, h := range level {
				out.Levels[l].Tables++
				out.Levels[l].Bytes += int64(h.meta.Size)
			}
		}
	}
	db.mu.Unlock()
	return out
}
