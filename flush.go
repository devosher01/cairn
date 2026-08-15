package cairn

import (
	"fmt"
	"slices"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
	"github.com/devosher01/cairn/internal/memtable"
	"github.com/devosher01/cairn/internal/sstable"
)

func (db *DB) runFlush() error {
	db.mu.Lock()
	imm, cutoff := db.imm, db.immCutoffWAL
	db.mu.Unlock()
	if imm == nil {
		return nil
	}

	num := db.nextFileNum.Add(1)
	meta, err := db.writeTable(num, memtableSource(imm))
	if err != nil {
		_ = db.fs.Remove(sstName(num))
		return err
	}
	if err := db.fs.SyncDir(); err != nil {
		_ = db.fs.Remove(sstName(num))
		return err
	}

	newState := db.state.Clone()
	newState.Levels[0] = append(newState.Levels[0], meta)
	newState.OldestWAL = cutoff
	newState.NextFileNum = db.nextFileNum.Load() + 1
	newState.LastSeq = keys.Seq(db.visible.Load())
	if err := manifest.Install(db.fs, newState); err != nil {
		_ = db.fs.Remove(sstName(num))
		return err
	}
	if err := db.openHandle(meta); err != nil {
		return err
	}
	db.installVersion(newState, nil)
	db.counters.flushes.Add(1)
	db.counters.flushBytes.Add(meta.Size)
	db.removeObsoleteWALs(cutoff)

	db.mu.Lock()
	db.imm = nil
	db.flushDone.Broadcast()
	db.stallEnd.Broadcast()
	db.mu.Unlock()
	return nil
}

func memtableSource(mem *memtable.Memtable) *mergeSource {
	return newMergeSource(mem.All(), func() error { return nil }, 0)
}

func (db *DB) writeTable(num uint64, sources ...*mergeSource) (manifest.Table, error) {
	m := newMergeIter(sources)
	w := db.newTableWriter(num)
	if w.err != nil {
		_ = m.close()
		return manifest.Table{}, w.err
	}
	for {
		ikey, value, ok := m.next()
		if !ok {
			break
		}
		if err := w.add(ikey, value); err != nil {
			_ = m.close()
			w.abort()
			return manifest.Table{}, err
		}
	}
	if err := m.close(); err != nil {
		w.abort()
		return manifest.Table{}, err
	}
	return w.finish()
}

func (db *DB) openHandle(meta manifest.Table) error {
	name := sstName(meta.FileNum)
	f, err := db.fs.Open(name)
	if err != nil {
		return err
	}
	size, err := f.Size()
	if err != nil {
		_ = f.Close()
		return err
	}
	if uint64(size) != meta.Size {
		_ = f.Close()
		return fmt.Errorf("%w: table %s is %d bytes, manifest records %d", ErrCorruption, name, size, meta.Size)
	}
	tbl, err := sstable.Open(f, size)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("%w: table %s: %w", ErrCorruption, name, err)
	}
	db.handles[meta.FileNum] = &tableHandle{meta: meta, tbl: tbl}
	return nil
}

func (db *DB) installVersion(state manifest.State, dropped []uint64) {
	var levels [manifest.NumLevels][]*tableHandle
	for l, tables := range state.Levels {
		for _, t := range tables {
			levels[l] = append(levels[l], db.handles[t.FileNum])
		}
	}
	slices.Reverse(levels[0])
	v := newVersion(db, levels)
	db.state = state
	for _, num := range dropped {
		db.handles[num].obsolete.Store(true)
		delete(db.handles, num)
	}

	db.mu.Lock()
	old := db.current
	db.current = v
	db.mu.Unlock()
	if old != nil {
		old.unref()
	}
}

func (db *DB) removeObsoleteWALs(cutoff uint64) {
	names, err := db.fs.List()
	if err != nil {
		return
	}
	for _, name := range names {
		if num, ok := walNumber(name); ok && num < cutoff {
			_ = db.fs.Remove(name)
		}
	}
}
