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
	"github.com/devosher01/cairn/internal/manifest"
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
	walNum     uint64
	failed     error

	mu           sync.Mutex
	mem          *memtable.Memtable
	imm          *memtable.Memtable
	immCutoffWAL uint64
	current      *version
	openHandles  int
	snaps        map[*Snapshot]struct{}
	closed       bool
	bgErr        error
	bgRetries    int
	flushDone    sync.Cond
	stallEnd     sync.Cond
	bgWake       sync.Cond
	bgDone       chan struct{}
	syncStop     chan struct{}
	syncDone     chan struct{}

	state         manifest.State
	handles       map[uint64]*tableHandle
	compactCursor [manifest.NumLevels][]byte

	nextFileNum atomic.Uint64
	visible     atomic.Uint64
	counters    counters
}

func (db *DB) Put(key, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	db.counters.puts.Add(1)
	return db.commit(func(b *batch.Batch) {
		b.Put(key, value)
	})
}

func (db *DB) Delete(key []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	db.counters.deletes.Add(1)
	return db.commit(func(b *batch.Batch) {
		b.Delete(key)
	})
}

func (db *DB) Get(key []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	db.counters.gets.Add(1)
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil, ErrClosed
	}
	mem, imm, v := db.mem, db.imm, db.current
	v.ref()
	db.mu.Unlock()
	defer v.unref()

	return db.getAt(key, keys.Seq(db.visible.Load()), mem, imm, v)
}

func (db *DB) getAt(key []byte, seq keys.Seq, mem, imm *memtable.Memtable, v *version) ([]byte, error) {
	if value, kind, ok := mem.Get(key, seq); ok {
		return getResult(value, kind)
	}
	if imm != nil {
		if value, kind, ok := imm.Get(key, seq); ok {
			return getResult(value, kind)
		}
	}
	value, kind, ok, err := v.get(key, seq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorruption, err)
	}
	if !ok || kind == keys.KindDelete {
		return nil, ErrNotFound
	}
	return bytes.Clone(value), nil
}

func getResult(value []byte, kind keys.Kind) ([]byte, error) {
	if kind == keys.KindDelete {
		return nil, ErrNotFound
	}
	return bytes.Clone(value), nil
}

func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	if db.openHandles > 0 {
		db.mu.Unlock()
		return ErrOpenHandles
	}
	db.closed = true
	db.bgWake.Broadcast()
	db.flushDone.Broadcast()
	db.stallEnd.Broadcast()
	db.mu.Unlock()

	if db.syncStop != nil {
		close(db.syncStop)
		<-db.syncDone
	}
	if db.bgDone != nil {
		<-db.bgDone
	}
	db.commitMu.Lock()
	err := db.wal.Close()
	db.commitMu.Unlock()

	db.mu.Lock()
	current := db.current
	db.mu.Unlock()
	current.unref()
	if lockErr := db.fileLock.Close(); lockErr != nil && err == nil {
		err = lockErr
	}
	return err
}

func (db *DB) intervalSyncLoop(ticker env.Ticker) {
	defer close(db.syncDone)
	for {
		select {
		case <-ticker.C():
			db.intervalSync()
		case <-db.syncStop:
			ticker.Stop()
			return
		}
	}
}

func (db *DB) intervalSync() {
	db.commitMu.Lock()
	defer db.commitMu.Unlock()
	db.mu.Lock()
	closed := db.closed
	db.mu.Unlock()
	if closed || db.failed != nil {
		return
	}
	if err := db.wal.Sync(); err != nil {
		db.failed = err
	}
}

func (db *DB) commit(fill func(*batch.Batch)) error {
	db.commitMu.Lock()
	defer db.commitMu.Unlock()
	db.writeBatch.Reset()
	fill(db.writeBatch)
	return db.commitLocked(db.writeBatch)
}

func (db *DB) commitLocked(bt *batch.Batch) error {
	if err := db.writable(); err != nil {
		return err
	}
	if err := db.waitForWriteRoom(); err != nil {
		return err
	}
	if bt.Count() == 0 {
		return nil
	}
	payload := bt.Seal(db.lastSeq + 1)
	if err := db.wal.Append(payload); err != nil {
		return db.fail(err)
	}
	db.counters.walBytesWritten.Add(uint64(len(payload)))
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

	if db.mem.Size() >= db.opts.MemtableSize {
		return db.rotate()
	}
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

func (db *DB) rotate() error {
	db.mu.Lock()
	if db.opts.DisableAutoCompaction && db.imm != nil {
		db.mu.Unlock()
		if err := db.runFlush(); err != nil {
			return db.fail(err)
		}
		db.mu.Lock()
	}
	for db.imm != nil && !db.closed && db.bgErr == nil {
		db.flushDone.Wait()
	}
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	if db.bgErr != nil {
		err := db.bgErr
		db.mu.Unlock()
		return fmt.Errorf("%w: %w", ErrDBFailed, err)
	}
	db.mu.Unlock()

	if err := db.wal.Close(); err != nil {
		return db.fail(err)
	}
	num := db.nextFileNum.Add(1)
	f, err := db.fs.Create(walName(num))
	if err != nil {
		return db.fail(err)
	}
	w, err := wal.NewWriter(f)
	if err != nil {
		return db.fail(err)
	}
	if err := db.fs.SyncDir(); err != nil {
		return db.fail(err)
	}

	db.mu.Lock()
	db.imm = db.mem
	db.immCutoffWAL = num
	db.mem = memtable.New(db.opts.Env.Rand)
	db.bgWake.Signal()
	db.mu.Unlock()

	db.wal = w
	db.walNum = num
	return nil
}

func (db *DB) waitForWriteRoom() error {
	if db.opts.DisableAutoCompaction {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	stalled := false
	for len(db.current.levels[0]) >= db.opts.L0StallTrigger && !db.closed && db.bgErr == nil {
		if !stalled {
			stalled = true
			db.counters.writeStalls.Add(1)
		}
		db.stallEnd.Wait()
	}
	if db.closed {
		return ErrClosed
	}
	if db.bgErr != nil {
		return fmt.Errorf("%w: %w", ErrDBFailed, db.bgErr)
	}
	return nil
}

func (db *DB) writable() error {
	db.mu.Lock()
	closed, bgErr := db.closed, db.bgErr
	db.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if bgErr != nil {
		return fmt.Errorf("%w: %w", ErrDBFailed, bgErr)
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
