package memtable_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

func TestMemtable_GetResolvesVisibleVersion(t *testing.T) {
	t.Parallel()

	m := memtable.New(newFakeRand(1))
	m.Insert(internalKey("k", 1, keys.KindSet), []byte("v1"))
	m.Insert(internalKey("k", 3, keys.KindSet), []byte("v3"))
	m.Insert(internalKey("k", 5, keys.KindDelete), nil)
	m.Insert(internalKey("solo", 2, keys.KindSet), []byte("s2"))
	m.Insert(internalKey("ab", 4, keys.KindSet), []byte("ab4"))

	tests := []struct {
		name      string
		giveUser  string
		giveSeq   keys.Seq
		wantValue string
		wantKind  keys.Kind
		wantOK    bool
	}{
		{name: "nothing is visible below the oldest version", giveUser: "k", giveSeq: 0},
		{name: "oldest version visible at its own seq", giveUser: "k", giveSeq: 1, wantValue: "v1", wantKind: keys.KindSet, wantOK: true},
		{name: "oldest version visible between writes", giveUser: "k", giveSeq: 2, wantValue: "v1", wantKind: keys.KindSet, wantOK: true},
		{name: "newer version wins at its own seq", giveUser: "k", giveSeq: 3, wantValue: "v3", wantKind: keys.KindSet, wantOK: true},
		{name: "newer version wins before the delete", giveUser: "k", giveSeq: 4, wantValue: "v3", wantKind: keys.KindSet, wantOK: true},
		{name: "delete visible at its own seq", giveUser: "k", giveSeq: 5, wantKind: keys.KindDelete, wantOK: true},
		{name: "delete visible above every version", giveUser: "k", giveSeq: 99, wantKind: keys.KindDelete, wantOK: true},
		{name: "single version key resolves", giveUser: "solo", giveSeq: 2, wantValue: "s2", wantKind: keys.KindSet, wantOK: true},
		{name: "single version key invisible below its seq", giveUser: "solo", giveSeq: 1},
		{name: "missing key is absent", giveUser: "missing", giveSeq: keys.MaxSeq},
		{name: "strict prefix of a stored key does not match", giveUser: "a", giveSeq: keys.MaxSeq},
		{name: "stored key matches in full", giveUser: "ab", giveSeq: keys.MaxSeq, wantValue: "ab4", wantKind: keys.KindSet, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, kind, ok := m.Get([]byte(tt.giveUser), tt.giveSeq)
			if ok != tt.wantOK || kind != tt.wantKind || string(value) != tt.wantValue {
				t.Errorf("Get(%q, %d) = (%q, %d, %t), want (%q, %d, %t)",
					tt.giveUser, tt.giveSeq, value, kind, ok, tt.wantValue, tt.wantKind, tt.wantOK)
			}
		})
	}
}

func TestMemtable_AllYieldsComparatorOrder(t *testing.T) {
	t.Parallel()

	inserts := []struct {
		user string
		seq  keys.Seq
	}{
		{user: "m", seq: 4},
		{user: "a", seq: 7},
		{user: "z", seq: 1},
		{user: "m", seq: 9},
		{user: "a", seq: 2},
		{user: "mm", seq: 3},
		{user: "z", seq: 8},
		{user: "a", seq: 5},
		{user: "b", seq: 6},
		{user: "m", seq: 10},
	}

	m := memtable.New(newFakeRand(42))
	for _, give := range inserts {
		m.Insert(internalKey(give.user, give.seq, keys.KindSet), []byte(give.user))
	}

	ikeys, values := collectAll(m)
	if !slices.IsSortedFunc(ikeys, keys.Compare) {
		t.Errorf("All yielded %x, want comparator order", ikeys)
	}
	if len(ikeys) != len(inserts) || len(values) != len(inserts) {
		t.Errorf("All yielded %d keys and %d values, want %d of each", len(ikeys), len(values), len(inserts))
	}
	if got := m.Len(); got != len(inserts) {
		t.Errorf("Len() = %d, want %d", got, len(inserts))
	}
	for i, ikey := range ikeys {
		if !bytes.Equal(keys.UserKey(ikey), values[i]) {
			t.Errorf("value for %x = %q, want %q", ikey, values[i], keys.UserKey(ikey))
		}
	}
}

func TestMemtable_ReinsertOfIdenticalEntryIsNoOp(t *testing.T) {
	t.Parallel()

	entries := []struct {
		user  string
		seq   keys.Seq
		value string
	}{
		{user: "a", seq: 1, value: "one"},
		{user: "b", seq: 2, value: "two"},
		{user: "a", seq: 3, value: "three"},
	}

	m := memtable.New(newFakeRand(3))
	for _, give := range entries {
		m.Insert(internalKey(give.user, give.seq, keys.KindSet), []byte(give.value))
	}
	wantKeys, wantValues := collectAll(m)
	wantLen, wantSize := m.Len(), m.Size()

	for _, give := range entries {
		m.Insert(internalKey(give.user, give.seq, keys.KindSet), []byte(give.value))
	}

	gotKeys, gotValues := collectAll(m)
	if !slices.EqualFunc(gotKeys, wantKeys, bytes.Equal) || !slices.EqualFunc(gotValues, wantValues, bytes.Equal) {
		t.Errorf("All after re-insert = (%x, %q), want (%x, %q)", gotKeys, gotValues, wantKeys, wantValues)
	}
	if got := m.Len(); got != wantLen {
		t.Errorf("Len() after re-insert = %d, want %d", got, wantLen)
	}
	if got := m.Size(); got != wantSize {
		t.Errorf("Size() after re-insert = %d, want %d", got, wantSize)
	}
}

func TestMemtable_InsertCopiesCallerBuffers(t *testing.T) {
	t.Parallel()

	m := memtable.New(newFakeRand(5))
	ikey := internalKey("key", 7, keys.KindSet)
	value := []byte("value")
	m.Insert(ikey, value)

	for i := range ikey {
		ikey[i] = 0xFF
	}
	for i := range value {
		value[i] = 0xFF
	}

	got, kind, ok := m.Get([]byte("key"), 7)
	if !ok || kind != keys.KindSet || string(got) != "value" {
		t.Errorf("Get after caller mutation = (%q, %d, %t), want (%q, %d, %t)", got, kind, ok, "value", keys.KindSet, true)
	}

	gotKeys, gotValues := collectAll(m)
	wantKey := internalKey("key", 7, keys.KindSet)
	if len(gotKeys) != 1 || !bytes.Equal(gotKeys[0], wantKey) || string(gotValues[0]) != "value" {
		t.Errorf("All after caller mutation = (%x, %q), want (%x, %q)", gotKeys, gotValues, [][]byte{wantKey}, "value")
	}
}

func TestMemtable_SizeGrowsWithEachInsert(t *testing.T) {
	t.Parallel()

	const entries = 8

	m := memtable.New(newFakeRand(11))
	if got := m.Size(); got != 0 {
		t.Errorf("Size() of an empty memtable = %d, want 0", got)
	}

	var previous int64
	for i := range entries {
		m.Insert(keys.Append(nil, numberedUser(i), keys.Seq(i+1), keys.KindSet), []byte(numberedValue(i)))
		got := m.Size()
		if got <= previous {
			t.Fatalf("Size() after %d inserts = %d, want more than %d", i+1, got, previous)
		}
		previous = got
	}

	m.Insert(keys.Append(nil, numberedUser(0), 1, keys.KindSet), []byte(numberedValue(0)))
	if got := m.Size(); got != previous {
		t.Errorf("Size() after re-insert = %d, want %d", got, previous)
	}
	if got := m.Len(); got != entries {
		t.Errorf("Len() = %d, want %d", got, entries)
	}
}

func TestMemtable_SameRandAndInsertsYieldSameContents(t *testing.T) {
	t.Parallel()

	const entries = 200

	first := memtable.New(newFakeRand(99))
	second := memtable.New(newFakeRand(99))
	for step := range entries {
		index := (step * 37) % entries
		ikey := keys.Append(nil, numberedUser(index), keys.Seq(step+1), keys.KindSet)
		value := []byte(numberedValue(index))
		first.Insert(ikey, value)
		second.Insert(ikey, value)
	}

	firstKeys, firstValues := collectAll(first)
	secondKeys, secondValues := collectAll(second)
	if !slices.EqualFunc(firstKeys, secondKeys, bytes.Equal) || !slices.EqualFunc(firstValues, secondValues, bytes.Equal) {
		t.Errorf("All outputs diverged: (%x, %q) and (%x, %q)", firstKeys, firstValues, secondKeys, secondValues)
	}
	if first.Len() != second.Len() || first.Len() != entries {
		t.Errorf("Len() = %d and %d, want %d for both", first.Len(), second.Len(), entries)
	}
}
