package cairn_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_stressKeys        = 64
	_stressWriterOps   = 3000
	_stressReaders     = 4
	_stressReaderOps   = 3000
	_stressWalks       = 12
	_stressSnapCycles  = 30
	_stressSnapReads   = 20
	_stressSnapSpan    = 8
	_stressDeleteEvery = 5
)

const (
	_stressValueLen   = 16
	_stressKeyField   = 4
	_stressCountField = 4
	_stressFiller     = 0x5a
)

const (
	_stressEnvSeed      uint64 = 20250814
	_stressReaderStream uint64 = 0xA0761D6478BD642F
	_stressMemtableSize int64  = 4096
)

func TestConcurrentReadersWritersAndMaintenance(t *testing.T) {
	t.Parallel()

	db, err := cairn.Open(_dbDir, stressOptions(simenv.New(_stressEnvSeed).Env()))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	final := make(map[string][]byte, _stressKeys)
	var wg sync.WaitGroup
	wg.Go(func() {
		stressWrite(t, db, final)
	})
	for reader := range _stressReaders {
		wg.Go(func() {
			stressRead(t, db, uint64(reader))
		})
	}
	wg.Go(func() {
		stressWalk(t, db)
	})
	wg.Go(func() {
		stressSnapshots(t, db)
	})
	wg.Wait()

	stressVerify(t, db, final)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func stressWrite(t *testing.T, db *cairn.DB, final map[string][]byte) {
	counters := make([]uint32, _stressKeys)
	for i := range _stressWriterOps {
		index := i % _stressKeys
		key := domainKey(index)
		if i%_stressDeleteEvery == _stressDeleteEvery-1 {
			if err := db.Delete([]byte(key)); err != nil {
				t.Errorf("write %d: Delete %s returned error: %v", i, key, err)

				return
			}
			delete(final, key)

			continue
		}
		counters[index]++
		value := stressValue(index, counters[index])
		if err := db.Put([]byte(key), value); err != nil {
			t.Errorf("write %d: Put %s returned error: %v", i, key, err)

			return
		}
		final[key] = value
	}
}

func stressRead(t *testing.T, db *cairn.DB, reader uint64) {
	rng := rand.New(rand.NewPCG(_stressEnvSeed+reader, _stressReaderStream))
	for i := range _stressReaderOps {
		key := domainKey(int(rng.Uint64() % _stressKeys))
		value, err := db.Get([]byte(key))
		if errors.Is(err, cairn.ErrNotFound) {
			continue
		}
		if err != nil {
			t.Errorf("reader %d op %d: Get %s returned error: %v", reader, i, key, err)

			return
		}
		if !stressWellFormed(value, key) {
			t.Errorf("reader %d op %d: Get %s = %x, want a value carrying key index %s",
				reader, i, key, value, key)

			return
		}
	}
}

func stressWalk(t *testing.T, db *cairn.DB) {
	for walk := range _stressWalks {
		it, err := db.NewIterator(cairn.IterOptions{})
		if err != nil {
			t.Errorf("walk %d: NewIterator returned error: %v", walk, err)

			return
		}
		if !stressWalkOnce(t, it, walk) {
			_ = it.Close()

			return
		}
		if err := it.Close(); err != nil {
			t.Errorf("walk %d: iterator Close returned error: %v", walk, err)

			return
		}
	}
}

func stressWalkOnce(t *testing.T, it *cairn.Iterator, walk int) bool {
	previous := ""
	for positioned := it.First(); positioned; positioned = it.Next() {
		key := string(it.Key())
		if previous != "" && key <= previous {
			t.Errorf("walk %d: yielded %s after %s, want strictly ascending keys", walk, key, previous)

			return false
		}
		if !stressWellFormed(it.Value(), key) {
			t.Errorf("walk %d: %s = %x, want a value carrying its own key index", walk, key, it.Value())

			return false
		}
		previous = key
	}
	if err := it.Error(); err != nil {
		t.Errorf("walk %d: iterator Error returned: %v", walk, err)

		return false
	}

	return true
}

func stressSnapshots(t *testing.T, db *cairn.DB) {
	for cycle := range _stressSnapCycles {
		snap, err := db.NewSnapshot()
		if err != nil {
			t.Errorf("cycle %d: NewSnapshot returned error: %v", cycle, err)

			return
		}
		stressSnapshotReads(t, snap, cycle)
		if err := snap.Close(); err != nil {
			t.Errorf("cycle %d: snapshot Close returned error: %v", cycle, err)

			return
		}
	}
}

func stressSnapshotReads(t *testing.T, snap *cairn.Snapshot, cycle int) {
	seen := make(map[string][]byte, _stressSnapSpan)
	present := make(map[string]bool, _stressSnapSpan)
	for read := range _stressSnapReads {
		key := domainKey((cycle + read) % _stressSnapSpan)
		value, err := snap.Get([]byte(key))
		if err != nil && !errors.Is(err, cairn.ErrNotFound) {
			t.Errorf("cycle %d read %d: snapshot Get %s returned error: %v", cycle, read, key, err)

			return
		}
		found := err == nil
		if found && !stressWellFormed(value, key) {
			t.Errorf("cycle %d read %d: snapshot Get %s = %x, want a value carrying its own key index",
				cycle, read, key, value)

			return
		}
		was, repeated := present[key]
		if !repeated {
			present[key], seen[key] = found, value

			continue
		}
		if was != found || !bytes.Equal(seen[key], value) {
			t.Errorf("cycle %d read %d: snapshot Get %s = %x found %t, the same snapshot first read %x found %t",
				cycle, read, key, value, found, seen[key], was)

			return
		}
	}
}

func stressVerify(t *testing.T, db *cairn.DB, final map[string][]byte) {
	t.Helper()

	for index := range _stressKeys {
		key := domainKey(index)
		want, written := final[key]
		got, err := db.Get([]byte(key))
		if !written {
			if !errors.Is(err, cairn.ErrNotFound) {
				t.Fatalf("after the run: Get %s error = %v, want %v", key, err, cairn.ErrNotFound)
			}

			continue
		}
		if err != nil {
			t.Fatalf("after the run: Get %s returned error: %v", key, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("after the run: Get %s = %x, want the last written %x", key, got, want)
		}
	}
}

func stressValue(index int, counter uint32) []byte {
	value := make([]byte, _stressValueLen)
	binary.BigEndian.PutUint32(value[:_stressKeyField], uint32(index))
	binary.BigEndian.PutUint32(value[_stressKeyField:_stressKeyField+_stressCountField], counter)
	for i := _stressKeyField + _stressCountField; i < len(value); i++ {
		value[i] = byte(counter) ^ _stressFiller
	}

	return value
}

func stressWellFormed(value []byte, key string) bool {
	if len(value) != _stressValueLen {
		return false
	}

	return domainKey(int(binary.BigEndian.Uint32(value[:_stressKeyField]))) == key
}

func stressOptions(sandbox env.Env) *cairn.Options {
	return &cairn.Options{
		Env:              sandbox,
		Sync:             cairn.SyncOff,
		MemtableSize:     _stressMemtableSize,
		BlockSize:        _modelBlockSize,
		L0CompactTrigger: _modelL0Compact,
		TargetFileSize:   _modelTargetFileSize,
		BaseLevelSize:    _modelBaseLevelSize,
	}
}
