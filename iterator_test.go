package cairn_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn"
)

const (
	_emptyIterSeed   uint64 = 8001
	_boundsSeed      uint64 = 8002
	_tombstoneSeed   uint64 = 8003
	_levelsSeed      uint64 = 8004
	_overwriteSeed   uint64 = 8005
	_seekSeed        uint64 = 8006
	_invalidIterSeed uint64 = 8007
	_pointInTimeSeed uint64 = 8008
	_iterCloseSeed   uint64 = 8009
)

func TestIterator_EmptyDatabaseYieldsNoEntries(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _emptyIterSeed)
	it := mustIterate(t, db, cairn.IterOptions{})

	if it.First() {
		t.Fatalf("First on an empty database = true, want false")
	}
	if it.Valid() {
		t.Fatalf("Valid on an empty database = true, want false")
	}
	if it.Next() {
		t.Fatalf("Next on an empty database = true, want false")
	}
	if err := it.Error(); err != nil {
		t.Fatalf("Error on an empty database returned: %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestIterator_BoundsAreHalfOpen(t *testing.T) {
	t.Parallel()

	entries := []string{"a=1", "b=2", "c=3", "d=4", "e=5"}

	tests := []struct {
		name      string
		giveLower string
		giveUpper string
		want      []string
	}{
		{name: "unbounded yields everything", want: []string{"a=1", "b=2", "c=3", "d=4", "e=5"}},
		{name: "lower bound is inclusive", giveLower: "c", want: []string{"c=3", "d=4", "e=5"}},
		{name: "upper bound is exclusive", giveUpper: "c", want: []string{"a=1", "b=2"}},
		{name: "both bounds", giveLower: "b", giveUpper: "d", want: []string{"b=2", "c=3"}},
		{name: "bounds falling between keys", giveLower: "bb", giveUpper: "dd", want: []string{"c=3", "d=4"}},
		{name: "equal bounds yield nothing", giveLower: "c", giveUpper: "c"},
		{name: "range past the last key", giveLower: "z"},
		{name: "range below the first key", giveUpper: "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := openManualDB(t, _boundsSeed)
			putAll(t, db, entries...)

			it := mustIterate(t, db, cairn.IterOptions{
				LowerBound: bound(tt.giveLower),
				UpperBound: bound(tt.giveUpper),
			})
			got := drain(t, it, it.First())
			if !slices.Equal(got, tt.want) {
				t.Fatalf("iteration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIterator_SkipsDeletedKeys(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _tombstoneSeed)
	putAll(t, db, "a=1", "b=2", "c=3")
	mustFlush(t, db)
	mustDelete(t, db, "b")
	putAll(t, db, "d=4")
	mustDelete(t, db, "d")

	it := mustIterate(t, db, cairn.IterOptions{})
	got := drain(t, it, it.First())
	want := []string{"a=1", "c=3"}
	if !slices.Equal(got, want) {
		t.Fatalf("iteration = %v, want %v", got, want)
	}
}

func TestIterator_MergesEntriesAcrossLevels(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _levelsSeed)
	putAll(t, db, "a=deep", "c=deep", "e=deep")
	mustFlush(t, db)
	putAll(t, db, "b=deep", "d=deep")
	mustFlush(t, db)
	mustCompact(t, db)
	putAll(t, db, "c=level0", "f=level0")
	mustFlush(t, db)
	putAll(t, db, "g=memtable")
	mustDelete(t, db, "a")

	if levelZeroTables(db) == 0 {
		t.Fatalf("level 0 holds no tables, want the last flush there")
	}
	if deepTables(db) == 0 {
		t.Fatalf("levels below 0 hold no tables, want the compaction output there")
	}

	it := mustIterate(t, db, cairn.IterOptions{})
	got := drain(t, it, it.First())
	want := []string{"b=deep", "c=level0", "d=deep", "e=deep", "f=level0", "g=memtable"}
	if !slices.Equal(got, want) {
		t.Fatalf("iteration = %v, want %v", got, want)
	}
}

func TestIterator_YieldsTheNewestValueOfAnOverwrittenKey(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _overwriteSeed)
	putAll(t, db, "k=first")
	mustFlush(t, db)
	putAll(t, db, "k=second")
	mustFlush(t, db)
	mustCompact(t, db)
	putAll(t, db, "k=third")

	it := mustIterate(t, db, cairn.IterOptions{})
	got := drain(t, it, it.First())
	want := []string{"k=third"}
	if !slices.Equal(got, want) {
		t.Fatalf("iteration = %v, want %v", got, want)
	}
}

func TestIterator_SeekGEPositionsAtTheFirstMatchingKey(t *testing.T) {
	t.Parallel()

	entries := []string{"b=2", "c=3", "d=4", "e=5"}

	tests := []struct {
		name      string
		giveLower string
		giveUpper string
		giveSeek  string
		want      []string
	}{
		{name: "seek below the first key", giveSeek: "a", want: []string{"b=2", "c=3", "d=4", "e=5"}},
		{name: "seek onto an existing key", giveSeek: "c", want: []string{"c=3", "d=4", "e=5"}},
		{name: "seek between keys", giveSeek: "cc", want: []string{"d=4", "e=5"}},
		{name: "seek past the last key", giveSeek: "z"},
		{name: "seek below the lower bound clamps to it", giveLower: "c", giveSeek: "a", want: []string{"c=3", "d=4", "e=5"}},
		{name: "seek above the upper bound yields nothing", giveUpper: "c", giveSeek: "d"},
		{name: "seek inside both bounds", giveLower: "b", giveUpper: "e", giveSeek: "c", want: []string{"c=3", "d=4"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := openManualDB(t, _seekSeed)
			putAll(t, db, entries...)

			it := mustIterate(t, db, cairn.IterOptions{
				LowerBound: bound(tt.giveLower),
				UpperBound: bound(tt.giveUpper),
			})
			got := drain(t, it, it.SeekGE([]byte(tt.giveSeek)))
			if !slices.Equal(got, tt.want) {
				t.Fatalf("iteration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIterator_KeyAndValuePanicWhenInvalid(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _invalidIterSeed)
	putAll(t, db, "a=1")

	it := mustIterate(t, db, cairn.IterOptions{})
	t.Cleanup(func() {
		_ = it.Close()
	})

	if !it.First() {
		t.Fatalf("First over one entry = false, want true")
	}
	if it.Next() {
		t.Fatalf("Next past the last entry = true, want false")
	}
	if it.Valid() {
		t.Fatalf("Valid past the last entry = true, want false")
	}
	mustPanic(t, "Key", func() {
		_ = it.Key()
	})
	mustPanic(t, "Value", func() {
		_ = it.Value()
	})
}

func TestIterator_IgnoresWritesMadeAfterCreation(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _pointInTimeSeed)
	putAll(t, db, "a=1", "c=3")

	it := mustIterate(t, db, cairn.IterOptions{})
	putAll(t, db, "b=2", "c=changed")
	mustDelete(t, db, "a")

	got := drain(t, it, it.First())
	want := []string{"a=1", "c=3"}
	if !slices.Equal(got, want) {
		t.Fatalf("iteration = %v, want %v", got, want)
	}
}

func TestIterator_SecondCloseReportsClosed(t *testing.T) {
	t.Parallel()

	db := openManualDB(t, _iterCloseSeed)
	putAll(t, db, "a=1")

	it := mustIterate(t, db, cairn.IterOptions{})
	if !it.First() {
		t.Fatalf("First over one entry = false, want true")
	}
	if err := it.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := it.Close(); !errors.Is(err, cairn.ErrClosed) {
		t.Fatalf("second Close error = %v, want %v", err, cairn.ErrClosed)
	}
	if it.First() {
		t.Fatalf("First on a closed iterator = true, want false")
	}
	if it.Valid() {
		t.Fatalf("Valid on a closed iterator = true, want false")
	}
}

func bound(key string) []byte {
	if key == "" {
		return nil
	}

	return []byte(key)
}
