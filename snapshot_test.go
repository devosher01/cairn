package cairn_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn"
)

const (
	_snapPinSeed      uint64 = 8101
	_snapIterSeed     uint64 = 8102
	_snapHandlesSeed  uint64 = 8103
	_iterHandlesSeed  uint64 = 8104
	_snapClosedSeed   uint64 = 8105
	_batchAtomicSeed  uint64 = 8106
	_batchInvalidSeed uint64 = 8107
	_batchLimitSeed   uint64 = 8108
)

const _batchMutations = 4

func TestSnapshot_ReadsPinnedValuesAcrossFlushAndCompaction(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _snapPinSeed)
	putAll(t, db, "k=old", "gone=present")

	snap := mustSnapshot(t, db)
	putAll(t, db, "k=new")
	mustDelete(t, db, "gone")
	mustFlush(t, db)
	putAll(t, db, "filler=1")
	mustFlush(t, db)
	mustCompact(t, db)

	if deep := countTables(db.TestingLevelFiles()[1:]); deep == 0 {
		t.Fatalf("levels below 0 hold no tables, want the compaction output there")
	}
	mustSnapGet(t, snap, "k", "old")
	mustSnapGet(t, snap, "gone", "present")
	mustGet(t, db, "k", "new")
	mustNotFound(t, db, "gone")

	if err := snap.Close(); err != nil {
		t.Fatalf("snapshot Close returned error: %v", err)
	}
	mustCompact(t, db)
	mustGet(t, db, "k", "new")
	mustNotFound(t, db, "gone")
}

func TestSnapshot_IteratorIgnoresLaterWrites(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _snapIterSeed)
	putAll(t, db, "a=1", "c=3")

	snap := mustSnapshot(t, db)
	t.Cleanup(func() {
		_ = snap.Close()
	})

	putAll(t, db, "b=2", "c=changed")
	mustDelete(t, db, "a")
	mustFlush(t, db)

	it, err := snap.NewIterator(cairn.IterOptions{})
	if err != nil {
		t.Fatalf("snapshot NewIterator returned error: %v", err)
	}
	got := drain(t, it, it.First())
	want := []string{"a=1", "c=3"}
	if !slices.Equal(got, want) {
		t.Fatalf("snapshot iteration = %v, want %v", got, want)
	}
}

func TestDB_CloseIsRefusedWhileASnapshotIsOpen(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _snapHandlesSeed)
	putAll(t, db, "k=v")
	snap := mustSnapshot(t, db)

	if err := db.Close(); !errors.Is(err, cairn.ErrOpenHandles) {
		t.Fatalf("Close holding a snapshot error = %v, want %v", err, cairn.ErrOpenHandles)
	}
	if err := snap.Close(); err != nil {
		t.Fatalf("snapshot Close returned error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close after releasing the snapshot returned error: %v", err)
	}
}

func TestDB_CloseIsRefusedWhileAnIteratorIsOpen(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _iterHandlesSeed)
	putAll(t, db, "k=v")
	it := mustIterate(t, db, cairn.IterOptions{})

	if err := db.Close(); !errors.Is(err, cairn.ErrOpenHandles) {
		t.Fatalf("Close holding an iterator error = %v, want %v", err, cairn.ErrOpenHandles)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("iterator Close returned error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close after releasing the iterator returned error: %v", err)
	}
}

func TestSnapshot_UseAfterCloseIsRejected(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _snapClosedSeed)
	putAll(t, db, "k=v")
	snap := mustSnapshot(t, db)

	if err := snap.Close(); err != nil {
		t.Fatalf("snapshot Close returned error: %v", err)
	}
	if err := snap.Close(); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("second snapshot Close error = %v, want %v", err, cairn.ErrClosed)
	}
	if _, err := snap.Get([]byte("k")); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("Get on a closed snapshot error = %v, want %v", err, cairn.ErrClosed)
	}
	if _, err := snap.NewIterator(cairn.IterOptions{}); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("NewIterator on a closed snapshot error = %v, want %v", err, cairn.ErrClosed)
	}
}

func TestBatch_WriteAppliesEveryMutationAtOnce(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _batchAtomicSeed)
	putAll(t, db, "d=old")

	batch := cairn.NewBatch()
	batch.Put([]byte("a"), []byte("1"))
	batch.Put([]byte("b"), []byte("2"))
	batch.Put([]byte("c"), []byte("3"))
	batch.Delete([]byte("d"))

	if got := batch.Count(); got != _batchMutations {
		t.Fatalf("batch Count = %d, want %d", got, _batchMutations)
	}
	mustNotFound(t, db, "a")
	mustNotFound(t, db, "b")
	mustNotFound(t, db, "c")
	mustGet(t, db, "d", "old")

	if err := db.Write(batch); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	mustGet(t, db, "a", "1")
	mustGet(t, db, "b", "2")
	mustGet(t, db, "c", "3")
	mustNotFound(t, db, "d")
}

func TestBatch_InvalidKeyRejectsTheWholeBatch(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _batchInvalidSeed)
	batch := cairn.NewBatch()
	batch.Put([]byte("before"), []byte("1"))
	batch.Put(nil, []byte("2"))
	batch.Put([]byte("after"), []byte("3"))

	if err := db.Write(batch); !errors.Is(err, cairn.ErrInvalidKey) {
		t.Fatalf("Write of a batch holding an empty key error = %v, want %v", err, cairn.ErrInvalidKey)
	}
	mustNotFound(t, db, "before")
	mustNotFound(t, db, "after")
}

func TestBatch_CountAboveTheLimitIsRejected(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _batchLimitSeed)
	batch := cairn.NewBatch()
	key := []byte("k")
	for range cairn.MaxBatchCount + 1 {
		batch.Put(key, nil)
	}

	if got := batch.Count(); got != cairn.MaxBatchCount+1 {
		t.Fatalf("batch Count = %d, want %d", got, cairn.MaxBatchCount+1)
	}
	if err := db.Write(batch); !errors.Is(err, cairn.ErrBatchTooLarge) {
		t.Fatalf("Write of an oversized batch error = %v, want %v", err, cairn.ErrBatchTooLarge)
	}
	mustNotFound(t, db, "k")
}

func mustSnapshot(t *testing.T, db *cairn.DB) *cairn.Snapshot {
	t.Helper()

	snap, err := db.NewSnapshot()
	if err != nil {
		t.Fatalf("NewSnapshot returned error: %v", err)
	}

	return snap
}

func mustSnapGet(t *testing.T, snap *cairn.Snapshot, key, want string) {
	t.Helper()

	got, err := snap.Get([]byte(key))
	if err != nil {
		t.Fatalf("snapshot Get %s returned error: %v", key, err)
	}
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("snapshot Get %s = %q, want %q", key, got, want)
	}
}
