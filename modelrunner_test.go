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
	_maxLiveSnaps = 3
	_seekEvery    = 3
)

const (
	_modelMemtableSize   int64 = 2048
	_modelTargetFileSize int64 = 4096
	_modelBaseLevelSize  int64 = 8192
	_modelBlockSize            = 512
	_modelL0Compact            = 2
)

type liveSnap struct {
	snap  *cairn.Snapshot
	state map[string][]byte
}

type runner struct {
	t       *testing.T
	seed    uint64
	mode    cairn.SyncMode
	sim     *simenv.Sim
	sandbox env.Env
	db      *cairn.DB
	oracle  *oracle
	snaps   []liveSnap
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
	case opScan:
		r.scan(index, o)
	case opBatch:
		r.batch(index, o)
	case opReopen:
		r.reopen(index, o)
	case opFlush:
		r.flush(index, o)
	case opCompact:
		r.compact(index, o)
	case opSnapCreate:
		r.snapCreate(index, o)
	case opSnapGet:
		r.snapGet(index, o)
	case opSnapScan:
		r.snapScan(index, o)
	case opSnapClose:
		r.snapClose(index, o)
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

func (r *runner) scan(index int, o op) {
	label := r.label(index, o)
	it, err := r.db.NewIterator(cairn.IterOptions{LowerBound: o.lo, UpperBound: o.hi})
	if err != nil {
		r.t.Fatalf("%s: NewIterator returned error: %v", label, err)
	}
	want := r.oracle.scan(o.lo, o.hi)
	if index%_seekEvery == 0 {
		positioned := it.SeekGE([]byte(o.key))
		r.comparePairs(label, r.collect(label, it, positioned), clipFrom(want, o.key))

		return
	}
	positioned := it.First()
	r.comparePairs(label, r.collect(label, it, positioned), want)
}

func (r *runner) batch(index int, o op) {
	label := r.label(index, o)
	b := cairn.NewBatch()
	for _, m := range o.mutations {
		if m.kind == histDelete {
			b.Delete([]byte(m.key))

			continue
		}
		b.Put([]byte(m.key), m.value)
	}
	if got := int(b.Count()); got != len(o.mutations) {
		r.t.Fatalf("%s: batch Count = %d, want %d", label, got, len(o.mutations))
	}
	if err := r.db.Write(b); err != nil {
		r.t.Fatalf("%s: Write returned error: %v", label, err)
	}
	r.oracle.apply(o.mutations)
}

func (r *runner) snapCreate(index int, o op) {
	label := r.label(index, o)
	if len(r.snaps) >= _maxLiveSnaps {
		r.closeOldestSnap(label)
	}
	snap, err := r.db.NewSnapshot()
	if err != nil {
		r.t.Fatalf("%s: NewSnapshot returned error: %v", label, err)
	}
	live := liveSnap{snap: snap, state: r.oracle.snapshotState()}
	r.snaps = append(r.snaps, live)

	r.put(index, o)
	r.checkSnapKey(label, live, o.key)
}

func (r *runner) snapGet(index int, o op) {
	if len(r.snaps) == 0 {
		return
	}
	r.checkSnapKey(r.label(index, o), r.snaps[index%len(r.snaps)], o.key)
}

func (r *runner) snapScan(index int, o op) {
	if len(r.snaps) == 0 {
		return
	}
	label := r.label(index, o)
	live := r.snaps[index%len(r.snaps)]
	it, err := live.snap.NewIterator(cairn.IterOptions{})
	if err != nil {
		r.t.Fatalf("%s: snapshot NewIterator returned error: %v", label, err)
	}
	positioned := it.First()
	r.comparePairs(label, r.collect(label, it, positioned), sortedPairs(live.state))
}

func (r *runner) snapClose(index int, o op) {
	if len(r.snaps) == 0 {
		return
	}
	r.closeOldestSnap(r.label(index, o))
}

func (r *runner) closeOldestSnap(label string) {
	live := r.snaps[0]
	r.snaps = r.snaps[1:]
	for i := range _keyDomain {
		r.checkSnapKey(label, live, domainKey(i))
	}
	if err := live.snap.Close(); err != nil {
		r.t.Fatalf("%s: snapshot Close returned error: %v", label, err)
	}
	if err := live.snap.Close(); !errors.Is(err, cairn.ErrClosed) {
		r.t.Fatalf("%s: second snapshot Close error = %v, want %v", label, err, cairn.ErrClosed)
	}
}

func (r *runner) releaseSnaps(label string) {
	if len(r.snaps) > 0 {
		if err := r.db.Close(); !errors.Is(err, cairn.ErrOpenHandles) {
			r.t.Fatalf("%s: Close holding %d snapshots error = %v, want %v",
				label, len(r.snaps), err, cairn.ErrOpenHandles)
		}
	}
	r.closeSnaps(label)
}

func (r *runner) closeSnaps(label string) {
	for len(r.snaps) > 0 {
		r.closeOldestSnap(label)
	}
}

func (r *runner) checkSnapKey(label string, live liveSnap, key string) {
	want, present := live.state[key]
	got, err := live.snap.Get([]byte(key))
	if !present {
		if !errors.Is(err, cairn.ErrNotFound) {
			r.t.Fatalf("%s: snapshot Get %s error = %v, want %v", label, key, err, cairn.ErrNotFound)
		}

		return
	}
	if err != nil {
		r.t.Fatalf("%s: snapshot Get %s returned error: %v", label, key, err)
	}
	if !bytes.Equal(got, want) {
		r.t.Fatalf("%s: snapshot Get %s = %d:%x, want %d:%x", label, key,
			len(got), got[:min(len(got), _opIndexBytes)],
			len(want), want[:min(len(want), _opIndexBytes)])
	}
}

func (r *runner) collect(label string, it *cairn.Iterator, positioned bool) []kv {
	out := make([]kv, 0, _keyDomain)
	for ; positioned; positioned = it.Next() {
		if !it.Valid() {
			r.t.Fatalf("%s: iterator invalid after %d entries while still positioned", label, len(out))
		}
		out = append(out, kv{key: string(it.Key()), value: bytes.Clone(it.Value())})
	}
	if it.Valid() {
		r.t.Fatalf("%s: iterator valid after the walk ended", label)
	}
	if err := it.Error(); err != nil {
		r.t.Fatalf("%s: iterator Error returned: %v", label, err)
	}
	if err := it.Close(); err != nil {
		r.t.Fatalf("%s: iterator Close returned error: %v", label, err)
	}

	return out
}

func (r *runner) comparePairs(label string, got, want []kv) {
	if !equalPairs(got, want) {
		r.t.Fatalf("%s: iteration = %s, want %s", label, formatPairs(got), formatPairs(want))
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
	r.releaseSnaps(label)
	if err := r.db.Close(); err != nil {
		r.t.Fatalf("%s: Close returned error: %v", label, err)
	}
	r.db = r.mustOpen(label)
	r.checkDomain(label)
	r.compactAndCheck(label)
}

func (r *runner) crash(index int, o op) {
	label := r.label(index, o)
	r.closeSnaps(label)
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
	r.closeSnaps(label)
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
	it, err := r.db.NewIterator(cairn.IterOptions{})
	if err != nil {
		r.t.Fatalf("%s: NewIterator for the full iteration returned error: %v", label, err)
	}
	positioned := it.First()
	r.comparePairs(label, r.collect(label, it, positioned), sortedPairs(r.oracle.state))
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
