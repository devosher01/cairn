package cairn_test

import (
	"bytes"
	"maps"
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
	o.history = append(o.history, histOp{kind: histPut, key: key, value: stored})
}

func (o *oracle) remove(key string) {
	delete(o.state, key)
	o.history = append(o.history, histOp{kind: histDelete, key: key})
}

func (o *oracle) get(key string) ([]byte, bool) {
	value, ok := o.state[key]

	return value, ok
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
		if maps.EqualFunc(dump, state, bytes.Equal) {
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
