package cairn_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const _dbDir = "db"

func openDB(t *testing.T, sandbox env.Env, mode cairn.SyncMode) *cairn.DB {
	t.Helper()

	db, err := cairn.Open(_dbDir, &cairn.Options{Env: sandbox, Sync: mode})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	return db
}

func openManualDB(t *testing.T, seed uint64) *cairn.DB {
	t.Helper()

	db, err := cairn.Open(_dbDir, &cairn.Options{
		Env:                   simenv.New(seed).Env(),
		Sync:                  cairn.SyncAlways,
		MemtableSize:          2048,
		BlockSize:             512,
		L0CompactTrigger:      2,
		TargetFileSize:        4096,
		BaseLevelSize:         8192,
		DisableAutoCompaction: true,
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func domainKey(index int) string {
	return fmt.Sprintf("key-%02d", index)
}

func putAll(t *testing.T, db *cairn.DB, entries ...string) {
	t.Helper()

	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("entry %q is not in key=value form", entry)
		}
		mustPut(t, db, key, value)
	}
}

func mustDelete(t *testing.T, db *cairn.DB, key string) {
	t.Helper()

	if err := db.Delete([]byte(key)); err != nil {
		t.Fatalf("Delete %s returned error: %v", key, err)
	}
}

func mustNotFound(t *testing.T, db *cairn.DB, key string) {
	t.Helper()

	if _, err := db.Get([]byte(key)); !errors.Is(err, cairn.ErrNotFound) {
		t.Fatalf("Get %s error = %v, want %v", key, err, cairn.ErrNotFound)
	}
}

func mustFlush(t *testing.T, db *cairn.DB) {
	t.Helper()

	if err := db.Flush(); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
}

func mustCompact(t *testing.T, db *cairn.DB) {
	t.Helper()

	if err := db.Compact(); err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}
}

func mustIterate(t *testing.T, db *cairn.DB, opts cairn.IterOptions) *cairn.Iterator {
	t.Helper()

	it, err := db.NewIterator(opts)
	if err != nil {
		t.Fatalf("NewIterator returned error: %v", err)
	}

	return it
}

func drain(t *testing.T, it *cairn.Iterator, positioned bool) []string {
	t.Helper()

	var out []string
	for ; positioned; positioned = it.Next() {
		if !it.Valid() {
			t.Fatalf("iterator invalid after %d entries while still positioned", len(out))
		}
		out = append(out, fmt.Sprintf("%s=%s", it.Key(), it.Value()))
	}
	if it.Valid() {
		t.Fatalf("iterator valid after the walk ended")
	}
	if err := it.Error(); err != nil {
		t.Fatalf("iterator Error returned: %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("iterator Close returned error: %v", err)
	}

	return out
}

func mustPanic(t *testing.T, name string, call func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Errorf("%s on an invalid iterator did not panic", name)
		}
	}()
	call()
}

func levelZeroTables(db *cairn.DB) int {
	return db.Metrics().Levels[0].Tables
}

func deepTables(db *cairn.DB) int {
	levels := db.Metrics().Levels
	total := 0
	for _, level := range levels[1:] {
		total += level.Tables
	}

	return total
}
