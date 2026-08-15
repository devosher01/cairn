package cairn

import (
	"bytes"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
	"github.com/devosher01/cairn/internal/sstable"
)

type rangeSource interface {
	seekGE(ikey []byte)
	first()
	next()
	valid() bool
	key() []byte
	value() []byte
	fail() error
}

type memSource struct {
	it *memtable.Iterator
}

func newMemSource(m *memtable.Memtable) *memSource {
	return &memSource{it: m.Iter()}
}

func (s *memSource) seekGE(ikey []byte) { s.it.SeekGE(ikey) }
func (s *memSource) first()             { s.it.First() }
func (s *memSource) next()              { s.it.Next() }
func (s *memSource) valid() bool        { return s.it.Valid() }
func (s *memSource) key() []byte        { return s.it.Key() }
func (s *memSource) value() []byte      { return s.it.Value() }
func (s *memSource) fail() error        { return nil }

type tableSource struct {
	it *sstable.TableIter
}

func newTableSource(t *sstable.Table) *tableSource {
	return &tableSource{it: t.Iter()}
}

func (s *tableSource) seekGE(ikey []byte) { s.it.SeekGE(ikey) }
func (s *tableSource) first()             { s.it.First() }
func (s *tableSource) next()              { s.it.Next() }
func (s *tableSource) valid() bool        { return s.it.Valid() }
func (s *tableSource) key() []byte        { return s.it.Key() }
func (s *tableSource) value() []byte      { return s.it.Value() }
func (s *tableSource) fail() error        { return s.it.Err() }

type levelSource struct {
	tables []*tableHandle
	at     int
	it     *sstable.TableIter
	err    error
}

func newLevelSource(tables []*tableHandle) *levelSource {
	return &levelSource{tables: tables, at: len(tables)}
}

func (s *levelSource) seekGE(ikey []byte) {
	s.err = nil
	lo, hi := 0, len(s.tables)
	for lo < hi {
		mid := (lo + hi) / 2
		if keys.Compare(s.tables[mid].meta.Largest, ikey) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	s.at = lo
	if s.at == len(s.tables) {
		s.it = nil
		return
	}
	s.it = s.tables[s.at].tbl.Iter()
	s.it.SeekGE(ikey)
	s.settle()
}

func (s *levelSource) first() {
	s.err = nil
	s.at = 0
	if len(s.tables) == 0 {
		s.it = nil
		return
	}
	s.it = s.tables[0].tbl.Iter()
	s.it.First()
	s.settle()
}

func (s *levelSource) next() {
	s.it.Next()
	s.settle()
}

func (s *levelSource) settle() {
	for {
		if s.it.Valid() {
			return
		}
		if err := s.it.Err(); err != nil {
			s.err = err
			s.it = nil
			return
		}
		s.at++
		if s.at >= len(s.tables) {
			s.it = nil
			return
		}
		s.it = s.tables[s.at].tbl.Iter()
		s.it.First()
	}
}

func (s *levelSource) valid() bool   { return s.it != nil && s.it.Valid() }
func (s *levelSource) key() []byte   { return s.it.Key() }
func (s *levelSource) value() []byte { return s.it.Value() }
func (s *levelSource) fail() error   { return s.err }

type sourceHeap struct {
	items []rangeSource
}

func (h *sourceHeap) init(sources []rangeSource) {
	h.items = h.items[:0]
	for _, s := range sources {
		if s.valid() {
			h.items = append(h.items, s)
		}
	}
	for i := len(h.items)/2 - 1; i >= 0; i-- {
		h.down(i)
	}
}

func (h *sourceHeap) top() rangeSource {
	return h.items[0]
}

func (h *sourceHeap) advanceTop() {
	h.items[0].next()
	if h.items[0].valid() {
		h.down(0)
		return
	}
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	h.items = h.items[:last]
	if len(h.items) > 0 {
		h.down(0)
	}
}

func (h *sourceHeap) less(i, j int) bool {
	return keys.Compare(h.items[i].key(), h.items[j].key()) < 0
}

func (h *sourceHeap) down(i int) {
	for {
		left, right := 2*i+1, 2*i+2
		smallest := i
		if left < len(h.items) && h.less(left, smallest) {
			smallest = left
		}
		if right < len(h.items) && h.less(right, smallest) {
			smallest = right
		}
		if smallest == i {
			return
		}
		h.items[i], h.items[smallest] = h.items[smallest], h.items[i]
		i = smallest
	}
}

func (h *sourceHeap) empty() bool {
	return len(h.items) == 0
}

func mergedSourcesFail(sources []rangeSource) error {
	for _, s := range sources {
		if err := s.fail(); err != nil {
			return err
		}
	}
	return nil
}

func shadowUser(h *sourceHeap, user []byte) {
	for !h.empty() && bytes.Equal(keys.UserKey(h.top().key()), user) {
		h.advanceTop()
	}
}
