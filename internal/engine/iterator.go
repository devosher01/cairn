package engine

import (
	"bytes"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

type IterOptions struct {
	LowerBound []byte
	UpperBound []byte
}

type Iterator struct {
	db      *DB
	v       *version
	seq     keys.Seq
	opts    IterOptions
	sources []rangeSource
	heap    sourceHeap
	seekBuf []byte
	curKey  []byte
	curVal  []byte
	valid   bool
	err     error
	closed  bool
}

func (db *DB) NewIterator(opts IterOptions) (*Iterator, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil, ErrClosed
	}
	return db.newIteratorLocked(opts, keys.Seq(db.visible.Load()), db.mem, db.imm, db.current), nil
}

func (s *Snapshot) NewIterator(opts IterOptions) (*Iterator, error) {
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.closed || s.db.closed {
		return nil, ErrClosed
	}
	return s.db.newIteratorLocked(opts, s.seq, s.mem, s.imm, s.v), nil
}

func (db *DB) newIteratorLocked(opts IterOptions, seq keys.Seq, mem, imm *memtable.Memtable, v *version) *Iterator {
	v.ref()
	db.openHandles++
	it := &Iterator{
		db:  db,
		v:   v,
		seq: seq,
		opts: IterOptions{
			LowerBound: bytes.Clone(opts.LowerBound),
			UpperBound: bytes.Clone(opts.UpperBound),
		},
	}
	it.sources = append(it.sources, newMemSource(mem))
	if imm != nil {
		it.sources = append(it.sources, newMemSource(imm))
	}
	for _, h := range v.levels[0] {
		it.sources = append(it.sources, newTableSource(h.tbl))
	}
	for _, level := range v.levels[1:] {
		if len(level) > 0 {
			it.sources = append(it.sources, newLevelSource(level))
		}
	}
	return it
}

func (it *Iterator) First() bool {
	if it.opts.LowerBound != nil {
		return it.SeekGE(it.opts.LowerBound)
	}
	if it.closed {
		return false
	}
	it.err = nil
	for _, s := range it.sources {
		s.first()
	}
	it.heap.init(it.sources)
	return it.settle()
}

func (it *Iterator) SeekGE(key []byte) bool {
	if it.closed {
		return false
	}
	it.err = nil
	target := key
	if it.opts.LowerBound != nil && bytes.Compare(target, it.opts.LowerBound) < 0 {
		target = it.opts.LowerBound
	}
	it.seekBuf = keys.AppendSeek(it.seekBuf[:0], target, it.seq)
	for _, s := range it.sources {
		s.seekGE(it.seekBuf)
	}
	it.heap.init(it.sources)
	return it.settle()
}

func (it *Iterator) Next() bool {
	if it.closed || !it.valid {
		return false
	}
	shadowUser(&it.heap, it.curKey)
	return it.settle()
}

func (it *Iterator) settle() bool {
	it.valid = false
	for !it.heap.empty() {
		top := it.heap.top()
		ikey := top.key()
		seq, kind := keys.Trailer(ikey)
		if seq > it.seq {
			it.heap.advanceTop()
			continue
		}
		user := keys.UserKey(ikey)
		if it.opts.UpperBound != nil && bytes.Compare(user, it.opts.UpperBound) >= 0 {
			break
		}
		if kind == keys.KindDelete {
			it.curKey = append(it.curKey[:0], user...)
			shadowUser(&it.heap, it.curKey)
			continue
		}
		it.curKey = append(it.curKey[:0], user...)
		it.curVal = append(it.curVal[:0], top.value()...)
		it.valid = true
		return true
	}
	if err := mergedSourcesFail(it.sources); err != nil {
		it.err = err
	}
	return false
}

func (it *Iterator) Valid() bool {
	return it.valid
}

func (it *Iterator) Key() []byte {
	if !it.valid {
		panic("cairn: Key on an invalid iterator")
	}
	return it.curKey
}

func (it *Iterator) Value() []byte {
	if !it.valid {
		panic("cairn: Value on an invalid iterator")
	}
	return it.curVal
}

func (it *Iterator) Error() error {
	return it.err
}

func (it *Iterator) Close() error {
	if it.closed {
		return ErrClosed
	}
	it.closed = true
	it.valid = false
	it.db.mu.Lock()
	it.db.openHandles--
	it.db.mu.Unlock()
	it.v.unref()
	return it.err
}
