package cairn

import (
	"errors"
	"fmt"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/memtable"
	"github.com/devosher01/cairn/internal/wal"
)

func Open(dir string, opts *Options) (*DB, error) {
	o, err := opts.resolved(dir)
	if err != nil {
		return nil, err
	}
	fileLock, err := o.Env.FS.Lock()
	if err != nil {
		if errors.Is(err, env.ErrLocked) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, dir)
		}
		return nil, err
	}
	db := &DB{
		opts:       o,
		fs:         o.Env.FS,
		fileLock:   fileLock,
		writeBatch: batch.New(),
		mem:        memtable.New(o.Env.Rand),
	}
	if err := db.recover(); err != nil {
		_ = fileLock.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) recover() error {
	wals, maxNum, err := walFiles(db.fs)
	if err != nil {
		return err
	}
	for _, name := range wals {
		if err := db.replayWAL(name); err != nil {
			return err
		}
	}
	f, err := db.fs.Create(walName(maxNum + 1))
	if err != nil {
		return err
	}
	w, err := wal.NewWriter(f)
	if err != nil {
		return err
	}
	if err := db.fs.SyncDir(); err != nil {
		return err
	}
	db.wal = w
	db.visible.Store(uint64(db.lastSeq))
	return nil
}

func (db *DB) replayWAL(name string) error {
	f, err := db.fs.Open(name)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	size, err := f.Size()
	if err != nil {
		return err
	}
	_, err = wal.Replay(f, size, func(payload []byte) error {
		last, applyErr := db.apply(payload)
		if applyErr != nil {
			return fmt.Errorf("%w: %s: %w", ErrCorruption, name, applyErr)
		}
		if last > db.lastSeq {
			db.lastSeq = last
		}
		return nil
	})
	return err
}
