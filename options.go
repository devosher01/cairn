package cairn

import (
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/osenv"
)

type SyncMode uint8

const (
	SyncAlways SyncMode = iota
	SyncOff
)

const (
	_defaultMemtableSize   = 4 << 20
	_defaultBlockSize      = 4 << 10
	_defaultBloomBits      = 10
	_defaultL0Compact      = 4
	_defaultL0Stall        = 12
	_defaultBaseLevelSize  = 10 << 20
	_defaultTargetFileSize = 4 << 20
)

type Options struct {
	Env                   env.Env
	Sync                  SyncMode
	MemtableSize          int64
	BlockSize             int
	BloomBitsPerKey       int
	L0CompactTrigger      int
	L0StallTrigger        int
	BaseLevelSize         int64
	TargetFileSize        int64
	DisableAutoCompaction bool
}

func (o *Options) resolved(dir string) (Options, error) {
	var out Options
	if o != nil {
		out = *o
	}
	if out.Env.FS == nil {
		real, err := osenv.New(dir)
		if err != nil {
			return Options{}, err
		}
		out.Env = real
	}
	if out.MemtableSize <= 0 {
		out.MemtableSize = _defaultMemtableSize
	}
	if out.BlockSize <= 0 {
		out.BlockSize = _defaultBlockSize
	}
	if out.BloomBitsPerKey <= 0 {
		out.BloomBitsPerKey = _defaultBloomBits
	}
	if out.L0CompactTrigger <= 0 {
		out.L0CompactTrigger = _defaultL0Compact
	}
	if out.L0StallTrigger <= 0 {
		out.L0StallTrigger = _defaultL0Stall
	}
	if out.BaseLevelSize <= 0 {
		out.BaseLevelSize = _defaultBaseLevelSize
	}
	if out.TargetFileSize <= 0 {
		out.TargetFileSize = _defaultTargetFileSize
	}
	return out, nil
}
