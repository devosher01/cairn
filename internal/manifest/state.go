package manifest

import (
	"bytes"

	"github.com/devosher01/cairn/internal/keys"
)

const NumLevels = 7

type Table struct {
	FileNum    uint64
	Size       uint64
	EntryCount uint64
	Smallest   []byte
	Largest    []byte
}

type State struct {
	NextFileNum uint64
	LastSeq     keys.Seq
	OldestWAL   uint64
	Levels      [NumLevels][]Table
}

func (s State) Clone() State {
	out := s
	for i, level := range s.Levels {
		if level == nil {
			continue
		}
		cloned := make([]Table, len(level))
		for j, t := range level {
			t.Smallest = bytes.Clone(t.Smallest)
			t.Largest = bytes.Clone(t.Largest)
			cloned[j] = t
		}
		out.Levels[i] = cloned
	}

	return out
}
