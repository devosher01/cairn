package memtable

import (
	"bytes"
	"iter"
	"sync/atomic"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
)

type Memtable struct {
	head   *node
	rnd    env.Rand
	height atomic.Int32
	size   atomic.Int64
	count  atomic.Int64
}

func New(rnd env.Rand) *Memtable {
	m := &Memtable{head: newHead(), rnd: rnd}
	m.height.Store(1)

	return m
}

func (m *Memtable) Insert(ikey, value []byte) {
	if len(ikey) < keys.TrailerSize {
		panic("memtable: internal key shorter than its trailer")
	}

	var prev [_maxHeight]*node
	for level := range prev {
		prev[level] = m.head
	}
	if found := m.findGE(ikey, &prev); found != nil && keys.Compare(found.ikey(), ikey) == 0 {
		return
	}

	n := newNode(ikey, value, m.randomHeight())
	m.link(n, &prev)
	m.size.Add(n.retained())
	m.count.Add(1)
}

func (m *Memtable) Get(user []byte, seq keys.Seq) ([]byte, keys.Kind, bool) {
	n := m.findGE(keys.AppendSeek(nil, user, seq), nil)
	if n == nil || !bytes.Equal(keys.UserKey(n.ikey()), user) {
		return nil, 0, false
	}

	_, kind := keys.Trailer(n.ikey())
	if kind == keys.KindDelete {
		return nil, kind, true
	}

	return n.value(), kind, true
}

func (m *Memtable) Size() int64 {
	return m.size.Load()
}

func (m *Memtable) Len() int {
	return int(m.count.Load())
}

func (m *Memtable) All() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for n := m.head.next[0].Load(); n != nil; n = n.next[0].Load() {
			if !yield(n.ikey(), n.value()) {
				return
			}
		}
	}
}
