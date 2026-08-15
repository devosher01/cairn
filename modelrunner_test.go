package cairn_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const _scatterSalt uint64 = 0xD1B54A32D192ED03

const _crashModeSpan = 3

const (
	_modelMemtableSize   int64 = 2048
	_modelTargetFileSize int64 = 4096
	_modelBaseLevelSize  int64 = 8192
	_modelBlockSize            = 512
	_modelL0Compact            = 2
)

type runner struct {
	t       *testing.T
	seed    uint64
	mode    cairn.SyncMode
	sim     *simenv.Sim
	sandbox env.Env
	db      *cairn.DB
	oracle  *oracle
	crashes int
}

func runSequence(t *testing.T, seed uint64, mode cairn.SyncMode) int {
	t.Helper()

	sim := simenv.New(seed)
	r := &runner{
		t:       t,
		seed:    seed,
		mode:    mode,
		sim:     sim,
		sandbox: sim.Env(),
		oracle:  newOracle(),
	}
	r.db = r.mustOpen(fmt.Sprintf("seed %d", seed))

	ops := generate(seed, _sequenceLength)
	for i, o := range ops {
		r.step(i, o)
	}
	r.finish(len(ops))

	return len(ops)
}

func (r *runner) step(index int, o op) {
	switch o.kind {
	case opPut:
		r.put(index, o)
	case opDelete:
		r.remove(index, o)
	case opGet:
		r.get(index, o)
	case opReopen:
		r.reopen(index, o)
	case opFlush:
		r.flush(index, o)
	case opCompact:
		r.compact(index, o)
	case opCrash:
		if r.crashes > 0 {
			r.reopen(index, op{kind: opReopen})

			return
		}
		r.crash(index, o)
	}
}

func (r *runner) put(index int, o op) {
	if err := r.db.Put([]byte(o.key), o.value); err != nil {
		r.t.Fatalf("%s: Put returned error: %v", r.label(index, o), err)
	}
	r.oracle.put(o.key, o.value)
}

func (r *runner) remove(index int, o op) {
	if err := r.db.Delete([]byte(o.key)); err != nil {
		r.t.Fatalf("%s: Delete returned error: %v", r.label(index, o), err)
	}
	r.oracle.remove(o.key)
}

func (r *runner) get(index int, o op) {
	want, present := r.oracle.get(o.key)
	got, err := r.db.Get([]byte(o.key))
	if !present {
		if !errors.Is(err, cairn.ErrNotFound) {
			r.t.Fatalf("%s: Get error = %v, want %v", r.label(index, o), err, cairn.ErrNotFound)
		}

		return
	}
	if err != nil {
		r.t.Fatalf("%s: Get returned error: %v", r.label(index, o), err)
	}
	if !bytes.Equal(got, want) {
		r.t.Fatalf("%s: Get = %d:%x, want %d:%x", r.label(index, o),
			len(got), got[:min(len(got), _opIndexBytes)],
			len(want), want[:min(len(want), _opIndexBytes)])
	}
}

func (r *runner) flush(index int, o op) {
	label := r.label(index, o)
	if err := r.db.TestingFlush(); err != nil {
		r.t.Fatalf("%s: TestingFlush returned error: %v", label, err)
	}
	r.checkDomain(label)
}

func (r *runner) compact(index int, o op) {
	r.compactAndCheck(r.label(index, o))
}

func (r *runner) reopen(index int, o op) {
	label := r.label(index, o)
	if err := r.db.Close(); err != nil {
		r.t.Fatalf("%s: Close returned error: %v", label, err)
	}
	r.db = r.mustOpen(label)
	r.checkDomain(label)
	r.compactAndCheck(label)
}

func (r *runner) crash(index int, o op) {
	label := r.label(index, o)
	point := simenv.CrashPoint{
		Op:          len(r.sim.Ops()),
		Mode:        crashModeFor(int(r.seed) + r.crashes),
		ScatterSeed: r.seed*_scatterSalt + uint64(index),
	}
	disk := r.sim.MaterializeCrash(point)
	r.crashes++
	r.sandbox = env.Env{FS: disk, Clock: r.sandbox.Clock, Rand: r.sandbox.Rand}

	db, err := cairn.Open(_dbDir, modelOptions(r.sandbox, r.mode))
	if err != nil {
		r.t.Fatalf("%s: Open after %s crash returned error: %v", label, crashModeName(point.Mode), err)
	}
	r.db = db

	r.verifyRecovery(label, point.Mode)
	r.compactAndCheck(label)
}

func (r *runner) verifyRecovery(label string, mode simenv.CrashMode) {
	dump := r.dump(label)
	if r.mode == cairn.SyncAlways {
		if !r.oracle.matches(dump) {
			r.t.Fatalf("%s: %s crash recovered %s, want every acknowledged write %s", label,
				crashModeName(mode), formatState(dump), formatState(r.oracle.state))
		}

		return
	}
	if !r.oracle.adoptPrefix(dump) {
		r.t.Fatalf("%s: %s crash recovered %s, which is no prefix of the %d acknowledged mutations ending in %s",
			label, crashModeName(mode), formatState(dump), r.oracle.acked(),
			formatState(r.oracle.state))
	}
}

func (r *runner) finish(count int) {
	label := fmt.Sprintf("seed %d after %d ops", r.seed, count)
	r.checkDomain(label)
	if err := r.db.Close(); err != nil {
		r.t.Fatalf("%s: Close returned error: %v", label, err)
	}
	if err := r.db.Close(); !errors.Is(err, cairn.ErrClosed) {
		r.t.Fatalf("%s: second Close error = %v, want %v", label, err, cairn.ErrClosed)
	}
}

func (r *runner) compactAndCheck(label string) {
	if err := r.db.TestingCompact(); err != nil {
		r.t.Fatalf("%s: TestingCompact returned error: %v", label, err)
	}
	r.checkDomain(label)
}

func (r *runner) checkDomain(label string) {
	dump := r.dump(label)
	if !r.oracle.matches(dump) {
		r.t.Fatalf("%s: full domain scan = %s, want %s", label, formatState(dump),
			formatState(r.oracle.state))
	}
}

func (r *runner) dump(label string) map[string][]byte {
	out := make(map[string][]byte, _keyDomain)
	for i := range _keyDomain {
		key := domainKey(i)
		value, err := r.db.Get([]byte(key))
		if errors.Is(err, cairn.ErrNotFound) {
			continue
		}
		if err != nil {
			r.t.Fatalf("%s: Get %s during full domain scan returned error: %v", label, key, err)
		}
		out[key] = value
	}

	return out
}

func (r *runner) mustOpen(label string) *cairn.DB {
	db, err := cairn.Open(_dbDir, modelOptions(r.sandbox, r.mode))
	if err != nil {
		r.t.Fatalf("%s: Open returned error: %v", label, err)
	}

	return db
}

func (r *runner) label(index int, o op) string {
	return fmt.Sprintf("seed %d op %d (%s)", r.seed, index, o)
}

func modelOptions(sandbox env.Env, mode cairn.SyncMode) *cairn.Options {
	return &cairn.Options{
		Env:                   sandbox,
		Sync:                  mode,
		MemtableSize:          _modelMemtableSize,
		BlockSize:             _modelBlockSize,
		L0CompactTrigger:      _modelL0Compact,
		TargetFileSize:        _modelTargetFileSize,
		BaseLevelSize:         _modelBaseLevelSize,
		DisableAutoCompaction: true,
	}
}

func crashModeFor(n int) simenv.CrashMode {
	switch n % _crashModeSpan {
	case 0:
		return simenv.CrashNone
	case 1:
		return simenv.CrashPrefix
	default:
		return simenv.CrashScatter
	}
}

func crashModeName(mode simenv.CrashMode) string {
	switch mode {
	case simenv.CrashNone:
		return "none"
	case simenv.CrashPrefix:
		return "prefix"
	default:
		return "scatter"
	}
}
