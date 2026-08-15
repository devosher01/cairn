package cairn

func (db *DB) TestingFlush() error {
	db.commitMu.Lock()
	if db.mem.Len() > 0 {
		if err := db.rotate(); err != nil {
			db.commitMu.Unlock()
			return err
		}
	}
	db.commitMu.Unlock()
	return db.runFlush()
}

func (db *DB) TestingCompact() error {
	for {
		level, ok := db.pickCompaction()
		if !ok {
			return nil
		}
		if err := db.runCompaction(level); err != nil {
			return err
		}
	}
}

func (db *DB) TestingLevelFiles() [][]uint64 {
	db.mu.Lock()
	defer db.mu.Unlock()
	out := make([][]uint64, len(db.current.levels))
	for l, level := range db.current.levels {
		for _, h := range level {
			out[l] = append(out[l], h.meta.FileNum)
		}
	}
	return out
}
