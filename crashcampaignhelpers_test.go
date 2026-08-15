package cairn_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_campaignDomain  = 16
	_campaignBatch   = 12
	_campaignRemoves = 2
	_campaignMinL0   = 2
	_campaignMinDeep = 1
)

const (
	_campaignMinValue  = 40
	_campaignValueSpan = 51
)

const (
	_campaignKeySalt   uint64 = 0x27BB2EE687B0B0FD
	_campaignValueSalt uint64 = 0x165667B19E3779F9
)

const (
	_campaignMemtableSize   int64 = 1024
	_campaignTargetFileSize int64 = 2048
	_campaignBaseLevelSize  int64 = 4096
	_campaignBlockSize            = 256
	_campaignL0Compact            = 2
)

type crashFloor struct {
	op    int
	acked int
}

type crashWorkload struct {
	t       *testing.T
	seed    uint64
	mode    cairn.SyncMode
	sim     *simenv.Sim
	sandbox env.Env
	db      *cairn.DB
	oracle  *oracle
	floors  []crashFloor
	states  []map[string][]byte
}

func runCrashWorkload(t *testing.T, seed uint64, mode cairn.SyncMode) *crashWorkload {
	t.Helper()

	sim := simenv.New(seed)
	w := &crashWorkload{
		t:       t,
		seed:    seed,
		mode:    mode,
		sim:     sim,
		sandbox: sim.Env(),
		oracle:  newOracle(),
	}

	db, err := cairn.Open(_dbDir, campaignOptions(w.sandbox, mode))
	if err != nil {
		t.Fatalf("%s: Open returned error: %v", w.label(0), err)
	}
	w.db = db
	w.script()
	w.states = crashStates(w.oracle)

	return w
}

func (w *crashWorkload) script() {
	step := w.puts(0, _campaignBatch)
	step = w.removes(step, _campaignRemoves)
	w.flush()
	step = w.puts(step, _campaignBatch)
	w.flush()
	w.requireL0(_campaignMinL0)
	w.compact()
	step = w.puts(step, _campaignBatch)
	step = w.removes(step, _campaignRemoves)
	w.flush()
	w.compact()
	w.requireDeep(_campaignMinDeep)
	w.puts(step, _campaignBatch)
	w.close()
}

func (w *crashWorkload) puts(step, count int) int {
	for range count {
		key := campaignKey(w.seed, step)
		value := campaignValue(w.seed, step)
		if err := w.db.Put([]byte(key), value); err != nil {
			w.t.Fatalf("%s: Put %s returned error: %v", w.label(step), key, err)
		}
		w.oracle.put(key, value)
		w.acknowledged()
		step++
	}

	return step
}

func (w *crashWorkload) removes(step, count int) int {
	for range count {
		key := w.presentKey(step)
		if err := w.db.Delete([]byte(key)); err != nil {
			w.t.Fatalf("%s: Delete %s returned error: %v", w.label(step), key, err)
		}
		w.oracle.remove(key)
		w.acknowledged()
		step++
	}

	return step
}

func (w *crashWorkload) flush() {
	if err := w.db.TestingFlush(); err != nil {
		w.t.Fatalf("%s: TestingFlush returned error: %v", w.label(w.oracle.acked()), err)
	}
	w.barrier()
}

func (w *crashWorkload) compact() {
	if err := w.db.TestingCompact(); err != nil {
		w.t.Fatalf("%s: TestingCompact returned error: %v", w.label(w.oracle.acked()), err)
	}
}

func (w *crashWorkload) close() {
	if err := w.db.Close(); err != nil {
		w.t.Fatalf("%s: Close returned error: %v", w.label(w.oracle.acked()), err)
	}
	w.barrier()
}

func (w *crashWorkload) requireL0(least int) {
	if got := len(w.db.TestingLevelFiles()[0]); got < least {
		w.t.Fatalf("%s: level 0 holds %d tables, want at least %d", w.label(w.oracle.acked()), got, least)
	}
}

func (w *crashWorkload) requireDeep(least int) {
	deep := 0
	for _, level := range w.db.TestingLevelFiles()[1:] {
		deep += len(level)
	}
	if deep < least {
		w.t.Fatalf("%s: levels 1 and deeper hold %d tables, want at least %d",
			w.label(w.oracle.acked()), deep, least)
	}
}

func (w *crashWorkload) acknowledged() {
	if w.mode == cairn.SyncAlways {
		w.barrier()
	}
}

func (w *crashWorkload) barrier() {
	w.floors = append(w.floors, crashFloor{op: len(w.sim.Ops()), acked: w.oracle.acked()})
}

func (w *crashWorkload) durableAt(op int) int {
	least := 0
	for _, f := range w.floors {
		if f.op > op {
			break
		}
		least = f.acked
	}

	return least
}

func (w *crashWorkload) prefixIndex(dump map[string][]byte, least int) (int, bool) {
	for k := least; k < len(w.states); k++ {
		if maps.EqualFunc(dump, w.states[k], bytes.Equal) {
			return k, true
		}
	}

	return 0, false
}

func (w *crashWorkload) presentKey(step int) string {
	names := slices.Sorted(maps.Keys(w.oracle.state))
	if len(names) == 0 {
		return campaignKey(w.seed, step)
	}

	return names[mix(w.seed+uint64(step))%uint64(len(names))]
}

func (w *crashWorkload) label(step int) string {
	return fmt.Sprintf("seed %d %s step %d", w.seed, campaignSyncName(w.mode), step)
}

func crashStates(o *oracle) []map[string][]byte {
	out := make([]map[string][]byte, 0, len(o.history)+1)
	state := make(map[string][]byte, _campaignDomain)
	out = append(out, maps.Clone(state))
	for _, h := range o.history {
		if h.kind == histDelete {
			delete(state, h.key)
		} else {
			state[h.key] = h.value
		}
		out = append(out, maps.Clone(state))
	}

	return out
}

func crashDump(t *testing.T, db *cairn.DB, label string) map[string][]byte {
	t.Helper()

	out := make(map[string][]byte, _campaignDomain)
	for i := range _campaignDomain {
		key := domainKey(i)
		value, err := db.Get([]byte(key))
		if errors.Is(err, cairn.ErrNotFound) {
			continue
		}
		if err != nil {
			t.Fatalf("%s: Get %s during full domain scan returned error: %v", label, key, err)
		}
		out[key] = value
	}

	return out
}

func campaignOptions(sandbox env.Env, mode cairn.SyncMode) *cairn.Options {
	return &cairn.Options{
		Env:                   sandbox,
		Sync:                  mode,
		MemtableSize:          _campaignMemtableSize,
		BlockSize:             _campaignBlockSize,
		L0CompactTrigger:      _campaignL0Compact,
		TargetFileSize:        _campaignTargetFileSize,
		BaseLevelSize:         _campaignBaseLevelSize,
		DisableAutoCompaction: true,
	}
}

func campaignKey(seed uint64, step int) string {
	return domainKey(int(mix(seed^(uint64(step)+_campaignKeySalt)) % _campaignDomain))
}

func campaignValue(seed uint64, step int) []byte {
	state := mix(seed ^ (uint64(step) + _campaignValueSalt))
	value := make([]byte, _campaignMinValue+int(state%_campaignValueSpan))
	binary.BigEndian.PutUint32(value[:_opIndexBytes], uint32(step))
	for i := _opIndexBytes; i < len(value); i++ {
		state = mix(state)
		value[i] = byte(state >> 33)
	}

	return value
}

func campaignSyncName(mode cairn.SyncMode) string {
	if mode == cairn.SyncAlways {
		return "SyncAlways"
	}

	return "SyncOff"
}
