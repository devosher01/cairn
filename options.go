package cairn

import (
	"time"

	"github.com/devosher01/cairn/internal/engine"
	"github.com/devosher01/cairn/internal/env"
)

type SyncMode uint8

const (
	SyncAlways SyncMode = iota
	SyncOff
	SyncInterval
)

type Options struct {
	Env                   env.Env
	Sync                  SyncMode
	Interval              time.Duration
	MemtableSize          int64
	BlockSize             int
	BloomBitsPerKey       int
	L0CompactTrigger      int
	L0StallTrigger        int
	BaseLevelSize         int64
	TargetFileSize        int64
	DisableAutoCompaction bool
}

func (o *Options) engine() *engine.Options {
	if o == nil {
		return nil
	}
	return &engine.Options{
		Env:                   o.Env,
		Sync:                  engine.SyncMode(o.Sync),
		Interval:              o.Interval,
		MemtableSize:          o.MemtableSize,
		BlockSize:             o.BlockSize,
		BloomBitsPerKey:       o.BloomBitsPerKey,
		L0CompactTrigger:      o.L0CompactTrigger,
		L0StallTrigger:        o.L0StallTrigger,
		BaseLevelSize:         o.BaseLevelSize,
		TargetFileSize:        o.TargetFileSize,
		DisableAutoCompaction: o.DisableAutoCompaction,
	}
}
