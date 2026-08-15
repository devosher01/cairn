package modeltest_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_fuzzOpPut     byte = 0
	_fuzzOpDelete  byte = 3
	_fuzzOpGet     byte = 4
	_fuzzOpFlush   byte = 5
	_fuzzOpCompact byte = 6
	_fuzzOpScan    byte = 7
	_fuzzOpKinds   byte = 8
)

const (
	_fuzzOpStride    = 2
	_fuzzMaxOps      = 128
	_fuzzKeySpan     = 16
	_fuzzValueMin    = 8
	_fuzzValueSpan   = 121
	_fuzzFlushRounds = 12
)

const _fuzzEnvSeed uint64 = 9001

func FuzzModelOps(f *testing.F) {
	for _, program := range fuzzSeedPrograms() {
		f.Add(program)
	}

	f.Fuzz(func(t *testing.T, program []byte) {
		runFuzzProgram(t, program)
	})
}

func fuzzSeedPrograms() [][]byte {
	everyOp := []byte{
		_fuzzOpPut, 1,
		_fuzzOpPut, 2,
		_fuzzOpDelete, 1,
		_fuzzOpGet, 1,
		_fuzzOpFlush, 0,
		_fuzzOpCompact, 0,
		_fuzzOpScan, 0,
	}
	deleteThenScan := []byte{
		_fuzzOpPut, 3,
		_fuzzOpPut, 4,
		_fuzzOpPut, 5,
		_fuzzOpDelete, 4,
		_fuzzOpScan, 0,
		_fuzzOpDelete, 3,
		_fuzzOpDelete, 5,
		_fuzzOpScan, 0,
	}
	flushHeavy := make([]byte, 0, 2*_fuzzFlushRounds*_fuzzOpStride)
	for round := range _fuzzFlushRounds {
		flushHeavy = append(flushHeavy, _fuzzOpPut, byte(round), _fuzzOpFlush, byte(round))
	}

	return [][]byte{everyOp, flushHeavy, deleteThenScan}
}

func runFuzzProgram(t *testing.T, program []byte) {
	t.Helper()

	db, err := cairn.Open(_dbDir, modelOptions(simenv.New(_fuzzEnvSeed).Env(), cairn.SyncOff))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	o := newOracle()
	ops := min(len(program)/_fuzzOpStride, _fuzzMaxOps)
	for i := range ops {
		at := i * _fuzzOpStride
		fuzzStep(t, db, o, i, program[at], program[at+1])
	}
	fuzzCheckDomain(t, db, o, fmt.Sprintf("after %d ops", ops))
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func fuzzStep(t *testing.T, db *cairn.DB, o *oracle, index int, code, arg byte) {
	t.Helper()

	label := fmt.Sprintf("op %d", index)
	key := domainKey(int(arg) % _fuzzKeySpan)
	switch code % _fuzzOpKinds {
	case _fuzzOpDelete:
		if err := db.Delete([]byte(key)); err != nil {
			t.Fatalf("%s: Delete %s returned error: %v", label, key, err)
		}
		o.remove(key)
	case _fuzzOpGet:
		fuzzCheckKey(t, db, o, label, key)
	case _fuzzOpFlush:
		if err := db.Flush(); err != nil {
			t.Fatalf("%s: Flush returned error: %v", label, err)
		}
	case _fuzzOpCompact:
		if err := db.Compact(); err != nil {
			t.Fatalf("%s: Compact returned error: %v", label, err)
		}
	case _fuzzOpScan:
		fuzzCheckScan(t, db, o, label)
	default:
		value := fuzzValue(code, arg)
		if err := db.Put([]byte(key), value); err != nil {
			t.Fatalf("%s: Put %s returned error: %v", label, key, err)
		}
		o.put(key, value)
	}
}

func fuzzCheckDomain(t *testing.T, db *cairn.DB, o *oracle, label string) {
	t.Helper()

	for i := range _keyDomain {
		fuzzCheckKey(t, db, o, label, domainKey(i))
	}
	fuzzCheckScan(t, db, o, label)
}

func fuzzCheckKey(t *testing.T, db *cairn.DB, o *oracle, label, key string) {
	t.Helper()

	want, present := o.get(key)
	got, err := db.Get([]byte(key))
	if !present {
		if !errors.Is(err, cairn.ErrNotFound) {
			t.Fatalf("%s: Get %s error = %v, want %v", label, key, err, cairn.ErrNotFound)
		}

		return
	}
	if err != nil {
		t.Fatalf("%s: Get %s returned error: %v", label, key, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: Get %s = %d:%x, want %d:%x", label, key,
			len(got), got[:min(len(got), _opIndexBytes)],
			len(want), want[:min(len(want), _opIndexBytes)])
	}
}

func fuzzCheckScan(t *testing.T, db *cairn.DB, o *oracle, label string) {
	t.Helper()

	it, err := db.NewIterator(cairn.IterOptions{})
	if err != nil {
		t.Fatalf("%s: NewIterator returned error: %v", label, err)
	}
	got, want := fuzzCollect(t, it, label), sortedPairs(o.state)
	if !equalPairs(got, want) {
		t.Fatalf("%s: full scan = %s, want %s", label, formatPairs(got), formatPairs(want))
	}
}

func fuzzCollect(t *testing.T, it *cairn.Iterator, label string) []kv {
	t.Helper()

	out := make([]kv, 0, _keyDomain)
	for positioned := it.First(); positioned; positioned = it.Next() {
		if !it.Valid() {
			t.Fatalf("%s: iterator invalid after %d entries while still positioned", label, len(out))
		}
		out = append(out, kv{key: string(it.Key()), value: bytes.Clone(it.Value())})
	}
	if it.Valid() {
		t.Fatalf("%s: iterator valid after the walk ended", label)
	}
	if err := it.Error(); err != nil {
		t.Fatalf("%s: iterator Error returned: %v", label, err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("%s: iterator Close returned error: %v", label, err)
	}

	return out
}

func fuzzValue(code, arg byte) []byte {
	value := make([]byte, _fuzzValueMin+int(arg)%_fuzzValueSpan)
	for i := range value {
		value[i] = code ^ arg ^ byte(i)
	}

	return value
}
