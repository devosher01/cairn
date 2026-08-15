package cairn

import (
	"errors"
	"fmt"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/manifest"
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
		handles:    make(map[uint64]*tableHandle),
		snaps:      make(map[*Snapshot]struct{}),
	}
	db.flushDone.L = &db.mu
	db.stallEnd.L = &db.mu
	db.bgWake.L = &db.mu
	if err := db.recover(); err != nil {
		for _, h := range db.handles {
			_ = h.tbl.Close()
		}
		_ = fileLock.Close()
		return nil, err
	}
	if !o.DisableAutoCompaction {
		db.bgDone = make(chan struct{})
		go db.backgroundWorker()
		db.wakeWorker()
	}
	return db, nil
}

func (db *DB) recover() error {
	state, exists, err := manifest.Load(db.fs)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCorruption, err)
	}
	names, err := db.fs.List()
	if err != nil {
		return err
	}
	if !exists {
		if dataFilesPresent(names) {
			return fmt.Errorf("%w: data files present without a manifest", ErrCorruption)
		}
		state = manifest.State{NextFileNum: 1}
		if err := manifest.Install(db.fs, state); err != nil {
			return err
		}
	}

	maxNum := state.NextFileNum
	referenced := make(map[uint64]bool)
	for _, level := range state.Levels {
		for _, t := range level {
			referenced[t.FileNum] = true
			maxNum = max(maxNum, t.FileNum)
		}
	}
	var replayWALs []string
	for _, name := range names {
		if num, ok := sstNumber(name); ok {
			maxNum = max(maxNum, num)
			if !referenced[num] {
				_ = db.fs.Remove(name)
			}
			continue
		}
		if num, ok := walNumber(name); ok {
			maxNum = max(maxNum, num)
			if num < state.OldestWAL {
				_ = db.fs.Remove(name)
			} else {
				replayWALs = append(replayWALs, name)
			}
			continue
		}
		if name == _manifestTmpName {
			_ = db.fs.Remove(name)
		}
	}

	for _, level := range state.Levels {
		for _, t := range level {
			if err := db.openHandle(t); err != nil {
				return err
			}
		}
	}
	db.installVersion(state, nil)
	db.nextFileNum.Store(maxNum)
	db.lastSeq = state.LastSeq

	sortWALs(replayWALs)
	for _, name := range replayWALs {
		if err := db.replayWAL(name); err != nil {
			return err
		}
	}

	num := db.nextFileNum.Add(1)
	f, err := db.fs.Create(walName(num))
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
	db.walNum = num
	db.immCutoffWAL = num
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

func dataFilesPresent(names []string) bool {
	for _, name := range names {
		if _, ok := sstNumber(name); ok {
			return true
		}
		if _, ok := walNumber(name); ok {
			return true
		}
	}
	return false
}
