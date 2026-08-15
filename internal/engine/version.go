package engine

import (
	"bytes"
	"sync/atomic"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
	"github.com/devosher01/cairn/internal/sstable"
)

type tableHandle struct {
	meta     manifest.Table
	tbl      *sstable.Table
	refs     atomic.Int32
	obsolete atomic.Bool
}

func (h *tableHandle) ref() {
	h.refs.Add(1)
}

func (h *tableHandle) unref(db *DB) {
	if h.refs.Add(-1) != 0 {
		return
	}
	_ = h.tbl.Close()
	if h.obsolete.Load() {
		_ = db.fs.Remove(sstName(h.meta.FileNum))
	}
}

type version struct {
	db     *DB
	levels [manifest.NumLevels][]*tableHandle
	refs   atomic.Int32
}

func newVersion(db *DB, levels [manifest.NumLevels][]*tableHandle) *version {
	v := &version{db: db, levels: levels}
	for _, level := range levels {
		for _, h := range level {
			h.ref()
		}
	}
	v.refs.Store(1)
	return v
}

func (v *version) ref() {
	v.refs.Add(1)
}

func (v *version) unref() {
	if v.refs.Add(-1) != 0 {
		return
	}
	for _, level := range v.levels {
		for _, h := range level {
			h.unref(v.db)
		}
	}
}

func (v *version) get(user []byte, seq keys.Seq) ([]byte, keys.Kind, bool, error) {
	for _, h := range v.levels[0] {
		if !handleCovers(h, user) {
			continue
		}
		value, kind, ok, err := h.tbl.Get(user, seq)
		if err != nil || ok {
			return value, kind, ok, err
		}
	}
	for _, level := range v.levels[1:] {
		h, ok := coveringTable(level, user)
		if !ok {
			continue
		}
		value, kind, ok, err := h.tbl.Get(user, seq)
		if err != nil || ok {
			return value, kind, ok, err
		}
	}
	return nil, 0, false, nil
}

func handleCovers(h *tableHandle, user []byte) bool {
	return bytes.Compare(user, keys.UserKey(h.meta.Smallest)) >= 0 &&
		bytes.Compare(user, keys.UserKey(h.meta.Largest)) <= 0
}

func coveringTable(level []*tableHandle, user []byte) (*tableHandle, bool) {
	lo, hi := 0, len(level)
	for lo < hi {
		mid := (lo + hi) / 2
		if bytes.Compare(keys.UserKey(level[mid].meta.Largest), user) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == len(level) {
		return nil, false
	}
	h := level[lo]
	if bytes.Compare(user, keys.UserKey(h.meta.Smallest)) < 0 {
		return nil, false
	}
	return h, true
}
