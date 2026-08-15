package cairn_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/wal"
)

const (
	_lockSeed       uint64 = 7001
	_closedSeed     uint64 = 7002
	_validationSeed uint64 = 7003
	_faultSeed      uint64 = 7004
	_durabilitySeed uint64 = 7005
	_corruptSeed    uint64 = 7006
)

const (
	_durableKeys       = 8
	_garbagePayloadLen = 5
	_corruptWALName    = "000099.wal"
)

var errInjected = errors.New("injected write failure")

func TestDB_SecondOpenOnTheSameEnvIsLocked(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_lockSeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)
	t.Cleanup(func() {
		_ = db.Close()
	})

	second, err := cairn.Open(_dbDir, &cairn.Options{Env: sim.Env(), Sync: cairn.SyncAlways})
	if !errors.Is(err, cairn.ErrLocked) {
		t.Fatalf("second Open error = %v, want %v", err, cairn.ErrLocked)
	}
	if second != nil {
		t.Fatalf("second Open returned a db alongside the error")
	}
}

func TestDB_OperationsAfterCloseAreRejected(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_closedSeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if err := db.Put([]byte("k"), []byte("v")); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("Put after Close error = %v, want %v", err, cairn.ErrClosed)
	}
	if err := db.Delete([]byte("k")); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("Delete after Close error = %v, want %v", err, cairn.ErrClosed)
	}
	if _, err := db.Get([]byte("k")); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("Get after Close error = %v, want %v", err, cairn.ErrClosed)
	}
	if err := db.Close(); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("second Close error = %v, want %v", err, cairn.ErrClosed)
	}
}

func TestDB_RejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	oversized := make([]byte, cairn.MaxKeySize+1)

	tests := []struct {
		name    string
		giveKey []byte
	}{
		{name: "empty key", giveKey: nil},
		{name: "key above the size limit", giveKey: oversized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(_validationSeed)
			db := openDB(t, sim.Env(), cairn.SyncAlways)
			t.Cleanup(func() {
				_ = db.Close()
			})

			if err := db.Put(tt.giveKey, []byte("v")); !errors.Is(err, cairn.ErrInvalidKey) {
				t.Fatalf("Put error = %v, want %v", err, cairn.ErrInvalidKey)
			}
			if err := db.Delete(tt.giveKey); !errors.Is(err, cairn.ErrInvalidKey) {
				t.Fatalf("Delete error = %v, want %v", err, cairn.ErrInvalidKey)
			}
			if _, err := db.Get(tt.giveKey); !errors.Is(err, cairn.ErrInvalidKey) {
				t.Fatalf("Get error = %v, want %v", err, cairn.ErrInvalidKey)
			}
		})
	}
}

func TestDB_RejectsOversizedValue(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_validationSeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)
	t.Cleanup(func() {
		_ = db.Close()
	})

	oversized := make([]byte, cairn.MaxValueSize+1)
	if err := db.Put([]byte("k"), oversized); !errors.Is(err, cairn.ErrInvalidValue) {
		t.Fatalf("Put error = %v, want %v", err, cairn.ErrInvalidValue)
	}
}

func TestDB_WriteFailureIsSticky(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_faultSeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := db.Put([]byte("alive"), []byte("value")); err != nil {
		t.Fatalf("Put before the fault returned error: %v", err)
	}

	sim.InjectFault(len(sim.Ops()), errInjected)

	if err := db.Put([]byte("doomed"), []byte("value")); !errors.Is(err, errInjected) {
		t.Fatalf("Put on the faulty WAL append error = %v, want %v", err, errInjected)
	}
	if err := db.Put([]byte("after"), []byte("value")); !errors.Is(err, cairn.ErrDBFailed) {
		t.Fatalf("Put after the failure error = %v, want %v", err, cairn.ErrDBFailed)
	}
	if err := db.Delete([]byte("alive")); !errors.Is(err, cairn.ErrDBFailed) {
		t.Fatalf("Delete after the failure error = %v, want %v", err, cairn.ErrDBFailed)
	}

	got, err := db.Get([]byte("alive"))
	if err != nil {
		t.Fatalf("Get after the failure returned error: %v", err)
	}
	if !bytes.Equal(got, []byte("value")) {
		t.Fatalf("Get after the failure = %q, want %q", got, "value")
	}
	if _, err := db.Get([]byte("doomed")); !errors.Is(err, cairn.ErrNotFound) {
		t.Fatalf("Get of the failed write error = %v, want %v", err, cairn.ErrNotFound)
	}
}

func TestDB_StateSurvivesReopen(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_durabilitySeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)

	for i := range _durableKeys {
		mustPut(t, db, domainKey(i), fmt.Sprintf("value-%02d", i))
	}
	mustPut(t, db, "overwritten", "first")
	mustPut(t, db, "overwritten", "second")
	mustPut(t, db, "overwritten", "third")
	mustPut(t, db, "removed", "present")
	if err := db.Delete([]byte("removed")); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened := openDB(t, sim.Env(), cairn.SyncAlways)
	t.Cleanup(func() {
		_ = reopened.Close()
	})

	for i := range _durableKeys {
		mustGet(t, reopened, domainKey(i), fmt.Sprintf("value-%02d", i))
	}
	mustGet(t, reopened, "overwritten", "third")
	if _, err := reopened.Get([]byte("removed")); !errors.Is(err, cairn.ErrNotFound) {
		t.Fatalf("Get of the deleted key error = %v, want %v", err, cairn.ErrNotFound)
	}
}

func TestDB_CorruptBatchInWALFailsOpen(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_corruptSeed)
	db := openDB(t, sim.Env(), cairn.SyncAlways)
	mustPut(t, db, "k", "v")
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	writeGarbageWAL(t, sim)

	reopened, err := cairn.Open(_dbDir, &cairn.Options{Env: sim.Env(), Sync: cairn.SyncAlways})
	if !errors.Is(err, cairn.ErrCorruption) {
		t.Fatalf("Open over a corrupt batch error = %v, want %v", err, cairn.ErrCorruption)
	}
	if reopened != nil {
		t.Fatalf("Open over a corrupt batch returned a db alongside the error")
	}
}

func writeGarbageWAL(t *testing.T, sim *simenv.Sim) {
	t.Helper()

	f, err := sim.Env().FS.Create(_corruptWALName)
	if err != nil {
		t.Fatalf("Create %s returned error: %v", _corruptWALName, err)
	}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter for %s returned error: %v", _corruptWALName, err)
	}
	if err := w.Append(bytes.Repeat([]byte{0xff}, _garbagePayloadLen)); err != nil {
		t.Fatalf("Append to %s returned error: %v", _corruptWALName, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close of %s returned error: %v", _corruptWALName, err)
	}
}

func mustPut(t *testing.T, db *cairn.DB, key, value string) {
	t.Helper()

	if err := db.Put([]byte(key), []byte(value)); err != nil {
		t.Fatalf("Put %s returned error: %v", key, err)
	}
}

func mustGet(t *testing.T, db *cairn.DB, key, want string) {
	t.Helper()

	got, err := db.Get([]byte(key))
	if err != nil {
		t.Fatalf("Get %s returned error: %v", key, err)
	}
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("Get %s = %q, want %q", key, got, want)
	}
}
