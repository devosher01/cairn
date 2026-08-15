package crashtest_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/crashtest"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_smallWriteOp  = 1
	_smallWriteLen = 4
	_largeWriteOp  = 4
	_largeWriteLen = 1500
	_failedWriteOp = 5
	_tinyWriteOp   = 6
	_opCount       = 7
)

func buildSim(t *testing.T) *simenv.Sim {
	t.Helper()

	sim := simenv.New(1)
	fsys := sim.Env().FS

	f, err := fsys.Create("a")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	mustWrite(t, f, make([]byte, _smallWriteLen))
	if err := fsys.SyncDir(); err != nil {
		t.Fatalf("SyncDir returned error: %v", err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	mustWrite(t, f, make([]byte, _largeWriteLen))

	sim.SetDiskBudget(3)
	if _, err := f.Write(make([]byte, 4)); !errors.Is(err, env.ErrNoSpace) {
		t.Fatalf("short write error = %v, want %v", err, env.ErrNoSpace)
	}
	sim.SetDiskBudget(-1)

	mustWrite(t, f, []byte{0x7f})

	if got := len(sim.Ops()); got != _opCount {
		t.Fatalf("op log has %d ops, want %d", got, _opCount)
	}

	return sim
}

func mustWrite(t *testing.T, f env.File, data []byte) {
	t.Helper()

	if _, err := f.Write(data); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
}

func collect(ops []simenv.Op, opts crashtest.Options) []simenv.CrashPoint {
	return slices.Collect(crashtest.Points(ops, opts))
}

func countMode(points []simenv.CrashPoint, mode simenv.CrashMode) int {
	count := 0
	for _, p := range points {
		if p.Mode == mode {
			count++
		}
	}

	return count
}

func slicesRange(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for v := lo; v <= hi; v++ {
		out = append(out, v)
	}

	return out
}

func tornCutsAt(points []simenv.CrashPoint, op int, mode simenv.CrashMode) []int {
	var out []int
	for _, p := range points {
		if p.Op == op && p.Torn > 0 && p.Mode == mode {
			out = append(out, p.Torn)
		}
	}

	return out
}
