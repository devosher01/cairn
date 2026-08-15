package cairn

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
	"github.com/devosher01/cairn/internal/wal"
)

type DB struct {
	opts     Options
	fs       env.FS
	fileLock io.Closer

	commitMu   sync.Mutex
	writeBatch *batch.Batch
	ikeyBuf    []byte
	lastSeq    keys.Seq
	wal        *wal.Writer
	failed     error

	mu     sync.Mutex
	mem    *memtable.Memtable
	closed bool

	visible atomic.Uint64
}

func (db *DB) Put(key, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	return db.commit(func(b *batch.Batch) {
		b.Put(key, value)
	})
}

func (db *DB) Delete(key []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return db.commit(func(b *batch.Batch) {
		b.Delete(key)
	})
}

func (db *DB) Get(key []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil, ErrClosed
	}
	mem := db.mem
	db.mu.Unlock()

	value, kind, ok := mem.Get(key, keys.Seq(db.visible.Load()))
	if !ok || kind == keys.KindDelete {
		return nil, ErrNotFound
	}
	return bytes.Clone(value), nil
}

func (db *DB) Close() error {
	db.commitMu.Lock()
	defer db.commitMu.Unlock()
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	db.closed = true
	db.mu.Unlock()

	err := db.wal.Close()
	if lockErr := db.fileLock.Close(); lockErr != nil && err == nil {
		err = lockErr
	}
	return err
}

func (db *DB) commit(fill func(*batch.Batch)) error {
	db.commitMu.Lock()
	defer db.commitMu.Unlock()

	if err := db.writable(); err != nil {
		return err
	}
	db.writeBatch.Reset()
	fill(db.writeBatch)
	if db.writeBatch.Count() == 0 {
		return nil
	}
	payload := db.writeBatch.Seal(db.lastSeq + 1)
	if err := db.wal.Append(payload); err != nil {
		return db.fail(err)
	}
	if db.opts.Sync == SyncAlways {
		if err := db.wal.Sync(); err != nil {
			return db.fail(err)
		}
	}
	last, err := db.apply(payload)
	if err != nil {
		panic(fmt.Sprintf("cairn: freshly sealed batch failed to decode: %v", err))
	}
	db.lastSeq = last
	db.visible.Store(uint64(last))
	return nil
}

func (db *DB) apply(payload []byte) (keys.Seq, error) {
	r, err := batch.NewReader(payload)
	if err != nil {
		return 0, err
	}
	var last keys.Seq
	for {
		e, ok := r.Next()
		if !ok {
			break
		}
		db.ikeyBuf = keys.Append(db.ikeyBuf[:0], e.Key, e.Seq, e.Kind)
		db.mem.Insert(db.ikeyBuf, e.Value)
		last = e.Seq
	}
	if err := r.Err(); err != nil {
		return 0, err
	}
	return last, nil
}

func (db *DB) writable() error {
	db.mu.Lock()
	closed := db.closed
	db.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if db.failed != nil {
		return fmt.Errorf("%w: %w", ErrDBFailed, db.failed)
	}
	return nil
}

func (db *DB) fail(err error) error {
	db.failed = err
	return err
}
