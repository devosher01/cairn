package modeltest_test

import (
	"bytes"
	"maps"
	"slices"
)

type histKind uint8

const (
	histPut histKind = iota + 1
	histDelete
)

type histOp struct {
	kind  histKind
	key   string
	value []byte
	tail  bool
}

type kv struct {
	key   string
	value []byte
}

type oracle struct {
	state   map[string][]byte
	history []histOp
}

func newOracle() *oracle {
	return &oracle{state: make(map[string][]byte, _keyDomain)}
}

func (o *oracle) put(key string, value []byte) {
	stored := bytes.Clone(value)
	o.state[key] = stored
	o.history = append(o.history, histOp{kind: histPut, key: key, value: stored, tail: true})
}

func (o *oracle) remove(key string) {
	delete(o.state, key)
	o.history = append(o.history, histOp{kind: histDelete, key: key, tail: true})
}

func (o *oracle) apply(mutations []histOp) {
	for i, m := range mutations {
		entry := histOp{kind: m.kind, key: m.key, tail: i == len(mutations)-1}
		if m.kind == histDelete {
			delete(o.state, m.key)
		} else {
			entry.value = bytes.Clone(m.value)
			o.state[m.key] = entry.value
		}
		o.history = append(o.history, entry)
	}
}

func (o *oracle) get(key string) ([]byte, bool) {
	value, ok := o.state[key]

	return value, ok
}

func (o *oracle) scan(lo, hi []byte) []kv {
	out := make([]kv, 0, len(o.state))
	for _, pair := range sortedPairs(o.state) {
		if lo != nil && pair.key < string(lo) {
			continue
		}
		if hi != nil && pair.key >= string(hi) {
			break
		}
		out = append(out, pair)
	}

	return out
}

func (o *oracle) snapshotState() map[string][]byte {
	clone := make(map[string][]byte, len(o.state))
	for key, value := range o.state {
		clone[key] = bytes.Clone(value)
	}

	return clone
}

func (o *oracle) acked() int {
	return len(o.history)
}

func (o *oracle) matches(dump map[string][]byte) bool {
	return maps.EqualFunc(dump, o.state, bytes.Equal)
}

func (o *oracle) adoptPrefix(dump map[string][]byte) bool {
	state := make(map[string][]byte, _keyDomain)
	for k, h := range o.history {
		if o.atomicBoundary(k) && maps.EqualFunc(dump, state, bytes.Equal) {
			o.state = state
			o.history = o.history[:k]

			return true
		}
		if h.kind == histDelete {
			delete(state, h.key)

			continue
		}
		state[h.key] = h.value
	}
	if !maps.EqualFunc(dump, state, bytes.Equal) {
		return false
	}
	o.state = state

	return true
}

func (o *oracle) atomicBoundary(index int) bool {
	return index == 0 || o.history[index-1].tail
}

func sortedPairs(state map[string][]byte) []kv {
	out := make([]kv, 0, len(state))
	for _, key := range slices.Sorted(maps.Keys(state)) {
		out = append(out, kv{key: key, value: state[key]})
	}

	return out
}
