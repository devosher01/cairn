package cairn

import "github.com/devosher01/cairn/internal/engine"

const NumLevels = engine.NumLevels

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
	Levels                 [NumLevels]LevelMetrics
}

func metricsFrom(m engine.Metrics) Metrics {
	out := Metrics{
		Puts:                   m.Puts,
		Deletes:                m.Deletes,
		Gets:                   m.Gets,
		WALBytesWritten:        m.WALBytesWritten,
		Flushes:                m.Flushes,
		FlushBytes:             m.FlushBytes,
		Compactions:            m.Compactions,
		CompactionBytesRead:    m.CompactionBytesRead,
		CompactionBytesWritten: m.CompactionBytesWritten,
		WriteStalls:            m.WriteStalls,
	}
	for i, level := range m.Levels {
		out.Levels[i] = LevelMetrics(level)
	}
	return out
}
