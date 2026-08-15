package modeltest_test

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	_dbDir        = "model"
	_keyDomain    = 24
	_opIndexBytes = 4
)

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
