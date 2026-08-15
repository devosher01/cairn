package modeltest_test

import (
	"bytes"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const _tickInterval = 40 * time.Millisecond

func TestDB_SyncIntervalMakesAckedWritesDurable(t *testing.T) {
	t.Parallel()

	sim := simenv.New(4200)
	db, err := cairn.Open("interval", &cairn.Options{
		Env:      sim.Env(),
		Sync:     cairn.SyncInterval,
		Interval: _tickInterval,
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	if err := db.Put([]byte("early"), []byte("v1")); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	unsyncedDisk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

	syncsBefore := countWALSyncs(sim)
	sim.Clock().Advance(_tickInterval)
	waitForWALSync(t, sim, syncsBefore)
	syncedDisk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	assertKeyState(t, sim, unsyncedDisk, "early", nil)
	assertKeyState(t, sim, syncedDisk, "early", []byte("v1"))
}

func countWALSyncs(sim *simenv.Sim) int {
	count := 0
	for _, op := range sim.Ops() {
		if op.Kind == simenv.OpSync {
			if _, ok := walNumberOf(op.Name); ok {
				count++
			}
		}
	}
	return count
}

func walNumberOf(name string) (string, bool) {
	if len(name) > 4 && name[len(name)-4:] == ".wal" {
		return name, true
	}
	return "", false
}

func waitForWALSync(t *testing.T, sim *simenv.Sim, before int) {
	t.Helper()

	for range 10_000_000 {
		if countWALSyncs(sim) > before {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("the interval sync never reached the write-ahead log")
}

func assertKeyState(t *testing.T, sim *simenv.Sim, disk *simenv.FS, key string, want []byte) {
	t.Helper()

	sandbox := env.Env{FS: disk, Clock: sim.Clock(), Rand: sim.Env().Rand}
	db, err := cairn.Open("interval", &cairn.Options{Env: sandbox, Sync: cairn.SyncOff})
	if err != nil {
		t.Fatalf("Open on the crash disk returned error: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	}()

	got, err := db.Get([]byte(key))
	if want == nil {
		if !errors.Is(err, cairn.ErrNotFound) {
			t.Fatalf("Get %s error = %v, want %v", key, err, cairn.ErrNotFound)
		}
		return
	}
	if err != nil {
		t.Fatalf("Get %s returned error: %v", key, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get %s = %q, want %q", key, got, want)
	}
}
