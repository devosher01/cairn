package cairn_test

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_dbDir        = "model"
	_keyDomain    = 24
	_opIndexBytes = 4
)

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

	db, err := cairn.Open(_dbDir, modelOptions(simenv.New(seed).Env(), cairn.SyncAlways))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
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

	if err := db.TestingFlush(); err != nil {
		t.Fatalf("TestingFlush returned error: %v", err)
	}
}

func mustCompact(t *testing.T, db *cairn.DB) {
	t.Helper()

	if err := db.TestingCompact(); err != nil {
		t.Fatalf("TestingCompact returned error: %v", err)
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

func countTables(levels [][]uint64) int {
	total := 0
	for _, level := range levels {
		total += len(level)
	}

	return total
}

func domainKey(index int) string {
	return fmt.Sprintf("key-%02d", index)
}

func formatState(state map[string][]byte) string {
	names := slices.Sorted(maps.Keys(state))
	out := make([]string, len(names))
	for i, name := range names {
		value := state[name]
		out[i] = fmt.Sprintf("%s=%d:%x", name, len(value), value[:min(len(value), _opIndexBytes)])
	}

	return "{" + strings.Join(out, " ") + "}"
}

func formatPairs(pairs []kv) string {
	out := make([]string, len(pairs))
	for i, pair := range pairs {
		out[i] = fmt.Sprintf("%s=%d:%x", pair.key, len(pair.value),
			pair.value[:min(len(pair.value), _opIndexBytes)])
	}

	return "[" + strings.Join(out, " ") + "]"
}

func equalPairs(got, want []kv) bool {
	return slices.EqualFunc(got, want, func(a, b kv) bool {
		return a.key == b.key && bytes.Equal(a.value, b.value)
	})
}

func clipFrom(pairs []kv, from string) []kv {
	for i, pair := range pairs {
		if pair.key >= from {
			return pairs[i:]
		}
	}

	return nil
}
