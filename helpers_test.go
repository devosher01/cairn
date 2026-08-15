package cairn_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
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
