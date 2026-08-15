package engine

const _maxTaskRetries = 3

func (db *DB) backgroundWorker() {
	defer close(db.bgDone)
	for {
		db.mu.Lock()
		for !db.closed && !db.hasWork() {
			db.bgIdle = true
			db.idleWait.Broadcast()
			db.bgWake.Wait()
		}
		db.bgIdle = false
		if db.closed {
			db.mu.Unlock()
			return
		}
		flush := db.imm != nil
		db.mu.Unlock()

		var err error
		if flush {
			err = db.runFlush()
		} else if level, ok := db.pickCompaction(); ok {
			err = db.runCompaction(level)
		}
		if err == nil {
			db.mu.Lock()
			db.bgRetries = 0
			db.mu.Unlock()
			continue
		}

		db.mu.Lock()
		db.bgRetries++
		if db.bgRetries >= _maxTaskRetries {
			db.bgErr = err
			db.flushDone.Broadcast()
			db.stallEnd.Broadcast()
			db.idleWait.Broadcast()
			db.mu.Unlock()
			return
		}
		db.mu.Unlock()
	}
}

func (db *DB) hasWork() bool {
	if db.imm != nil {
		return true
	}
	_, ok := db.pickCompaction()
	return ok
}

func (db *DB) wakeWorker() {
	db.mu.Lock()
	db.bgWake.Signal()
	db.mu.Unlock()
}
