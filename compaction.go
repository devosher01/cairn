package cairn

import (
	"bytes"
	"slices"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

func (db *DB) pickCompaction() (int, bool) {
	bestLevel, bestScore := -1, 1.0
	if score := float64(len(db.state.Levels[0])) / float64(db.opts.L0CompactTrigger); score >= bestScore {
		bestLevel, bestScore = 0, score
	}
	for level := 1; level < manifest.NumLevels-1; level++ {
		var total int64
		for _, t := range db.state.Levels[level] {
			total += int64(t.Size)
		}
		if score := float64(total) / float64(db.levelTarget(level)); score >= bestScore {
			bestLevel, bestScore = level, score
		}
	}
	return bestLevel, bestLevel >= 0
}

func (db *DB) levelTarget(level int) int64 {
	target := db.opts.BaseLevelSize
	for range level - 1 {
		target *= 10
	}
	return target
}

func (db *DB) runCompaction(level int) error {
	inputs, target := db.selectInputs(level)
	if len(inputs) == 0 {
		return nil
	}

	outputs, read, err := db.mergeInputs(inputs, target)
	if err != nil {
		for _, t := range outputs {
			_ = db.fs.Remove(sstName(t.FileNum))
		}
		return err
	}
	if err := db.fs.SyncDir(); err != nil {
		for _, t := range outputs {
			_ = db.fs.Remove(sstName(t.FileNum))
		}
		return err
	}

	newState := db.state.Clone()
	dropped := make([]uint64, 0, len(inputs))
	for _, in := range inputs {
		dropped = append(dropped, in.FileNum)
	}
	newState.Levels[level] = removeTables(newState.Levels[level], dropped)
	newState.Levels[target] = removeTables(newState.Levels[target], dropped)
	newState.Levels[target] = insertTables(newState.Levels[target], outputs)
	newState.NextFileNum = db.nextFileNum.Load() + 1
	newState.LastSeq = keys.Seq(db.visible.Load())
	if err := manifest.Install(db.fs, newState); err != nil {
		for _, t := range outputs {
			_ = db.fs.Remove(sstName(t.FileNum))
		}
		return err
	}
	for _, t := range outputs {
		if err := db.openHandle(t); err != nil {
			return err
		}
	}
	if level > 0 && len(db.state.Levels[level]) > 0 {
		db.compactCursor[level] = bytes.Clone(inputs[0].Largest)
	}
	db.installVersion(newState, dropped)
	db.counters.compactions.Add(1)
	db.counters.compactionBytesRead.Add(read)
	var written uint64
	for _, t := range outputs {
		written += t.Size
	}
	db.counters.compactionBytesWritten.Add(written)

	db.mu.Lock()
	db.stallEnd.Broadcast()
	db.mu.Unlock()
	return nil
}

func (db *DB) selectInputs(level int) ([]manifest.Table, int) {
	if level == 0 {
		inputs := slices.Clone(db.state.Levels[0])
		if len(inputs) == 0 {
			return nil, 1
		}
		smallest, largest := keyRange(inputs)
		inputs = append(inputs, overlapping(db.state.Levels[1], smallest, largest)...)
		return inputs, 1
	}

	tables := db.state.Levels[level]
	if len(tables) == 0 {
		return nil, level + 1
	}
	pick := tables[0]
	if cursor := db.compactCursor[level]; cursor != nil {
		for _, t := range tables {
			if keys.Compare(t.Smallest, cursor) > 0 {
				pick = t
				break
			}
		}
	}
	inputs := []manifest.Table{pick}
	smallest, largest := keyRange(inputs)
	inputs = append(inputs, overlapping(db.state.Levels[level+1], smallest, largest)...)
	return inputs, level + 1
}

func (db *DB) mergeInputs(inputs []manifest.Table, target int) ([]manifest.Table, uint64, error) {
	sources := make([]*mergeSource, 0, len(inputs))
	var read uint64
	for i, in := range inputs {
		h := db.handles[in.FileNum]
		sources = append(sources, newMergeSource(h.tbl.All(), h.tbl.AllErr, i))
		read += in.Size
	}
	m := newMergeIter(sources)
	oldestSnap := db.oldestSnapshotSeq()

	var outputs []manifest.Table
	var w *tableWriter
	var prevUser []byte
	var lastSeqForKey keys.Seq
	prevSet := false
	fail := func(err error) ([]manifest.Table, uint64, error) {
		_ = m.close()
		if w != nil {
			w.abort()
		}
		return outputs, read, err
	}
	for {
		ikey, value, ok := m.next()
		if !ok {
			break
		}
		user := keys.UserKey(ikey)
		seq, kind := keys.Trailer(ikey)
		sameUser := prevSet && bytes.Equal(user, prevUser)
		if !sameUser {
			prevUser = append(prevUser[:0], user...)
			prevSet = true
			lastSeqForKey = keys.MaxSeq
		}
		shadowed := lastSeqForKey <= oldestSnap
		lastSeqForKey = seq
		if sameUser && shadowed {
			continue
		}
		if kind == keys.KindDelete && seq <= oldestSnap && db.bottommostFor(user, target) {
			continue
		}

		if w == nil {
			w = db.newTableWriter(db.nextFileNum.Add(1))
			if w.err != nil {
				return fail(w.err)
			}
		}
		if err := w.add(ikey, value); err != nil {
			return fail(err)
		}
		if w.size() >= db.opts.TargetFileSize {
			t, err := w.finish()
			if err != nil {
				return fail(err)
			}
			outputs = append(outputs, t)
			w = nil
		}
	}
	if err := m.close(); err != nil {
		if w != nil {
			w.abort()
		}
		return outputs, read, err
	}
	if w != nil {
		t, err := w.finish()
		if err != nil {
			return outputs, read, err
		}
		outputs = append(outputs, t)
	}
	return outputs, read, nil
}

func (db *DB) bottommostFor(user []byte, target int) bool {
	for level := target + 1; level < manifest.NumLevels; level++ {
		for _, t := range db.state.Levels[level] {
			if bytes.Compare(user, keys.UserKey(t.Smallest)) >= 0 &&
				bytes.Compare(user, keys.UserKey(t.Largest)) <= 0 {
				return false
			}
		}
	}
	return true
}

func keyRange(tables []manifest.Table) ([]byte, []byte) {
	smallest, largest := tables[0].Smallest, tables[0].Largest
	for _, t := range tables[1:] {
		if keys.Compare(t.Smallest, smallest) < 0 {
			smallest = t.Smallest
		}
		if keys.Compare(t.Largest, largest) > 0 {
			largest = t.Largest
		}
	}
	return smallest, largest
}

func overlapping(tables []manifest.Table, smallest, largest []byte) []manifest.Table {
	var out []manifest.Table
	for _, t := range tables {
		if bytes.Compare(keys.UserKey(t.Largest), keys.UserKey(smallest)) < 0 ||
			bytes.Compare(keys.UserKey(t.Smallest), keys.UserKey(largest)) > 0 {
			continue
		}
		out = append(out, t)
	}
	return out
}

func removeTables(tables []manifest.Table, nums []uint64) []manifest.Table {
	return slices.DeleteFunc(tables, func(t manifest.Table) bool {
		return slices.Contains(nums, t.FileNum)
	})
}

func insertTables(tables []manifest.Table, add []manifest.Table) []manifest.Table {
	tables = append(tables, add...)
	slices.SortFunc(tables, func(a, b manifest.Table) int {
		return keys.Compare(a.Smallest, b.Smallest)
	})
	return tables
}
