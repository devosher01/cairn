package cairn

import (
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

type Snapshot struct {
	db     *DB
	seq    keys.Seq
	v      *version
	mem    *memtable.Memtable
	imm    *memtable.Memtable
	closed bool
}

func (db *DB) NewSnapshot() (*Snapshot, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil, ErrClosed
	}
	s := &Snapshot{
		db:  db,
		seq: keys.Seq(db.visible.Load()),
		v:   db.current,
		mem: db.mem,
		imm: db.imm,
	}
	s.v.ref()
	db.openHandles++
	db.snaps[s] = struct{}{}
	return s, nil
}

func (s *Snapshot) Get(key []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if s.closed {
		return nil, ErrClosed
	}
	return s.db.getAt(key, s.seq, s.mem, s.imm, s.v)
}

func (s *Snapshot) Close() error {
	s.db.mu.Lock()
	if s.closed {
		s.db.mu.Unlock()
		return ErrClosed
	}
	s.closed = true
	s.db.openHandles--
	delete(s.db.snaps, s)
	s.db.mu.Unlock()
	s.v.unref()
	return nil
}

func (db *DB) oldestSnapshotSeq() keys.Seq {
	db.mu.Lock()
	defer db.mu.Unlock()
	oldest := keys.MaxSeq
	for s := range db.snaps {
		oldest = min(oldest, s.seq)
	}
	return oldest
}
