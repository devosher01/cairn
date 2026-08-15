package memtable_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

func internalKey(user string, seq keys.Seq, kind keys.Kind) []byte {
	return keys.Append(nil, []byte(user), seq, kind)
}

func collectAll(m *memtable.Memtable) ([][]byte, [][]byte) {
	var ikeys, values [][]byte
	for ikey, value := range m.All() {
		ikeys = append(ikeys, slices.Clone(ikey))
		values = append(values, slices.Clone(value))
	}

	return ikeys, values
}

func collectIter(it *memtable.Iterator) ([][]byte, [][]byte) {
	var ikeys, values [][]byte
	for it.First(); it.Valid(); it.Next() {
		ikeys = append(ikeys, slices.Clone(it.Key()))
		values = append(values, slices.Clone(it.Value()))
	}

	return ikeys, values
}

func assertPanics(t *testing.T, name string, op func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic, want a panic", name)
		}
	}()

	op()
}

func numberedUser(index int) []byte {
	return fmt.Appendf(nil, "key%05d", index)
}

func numberedValue(index int) string {
	return fmt.Sprintf("value%05d", index)
}
