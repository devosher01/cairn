package memtable_test

import (
	"bytes"
	"slices"
	"sync"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

func TestIterator_SeekGEPositionsAtFirstGreaterOrEqualKey(t *testing.T) {
	t.Parallel()

	const entrySeq keys.Seq = 2

	m := memtable.New(newFakeRand(17))
	for _, user := range []string{"b", "d", "f"} {
		m.Insert(internalKey(user, entrySeq, keys.KindSet), []byte(user))
	}

	tests := []struct {
		name      string
		giveUser  string
		giveSeq   keys.Seq
		wantUser  string
		wantValid bool
	}{
		{name: "exact key lands on itself", giveUser: "d", giveSeq: entrySeq, wantUser: "d", wantValid: true},
		{name: "key between entries lands on the next entry", giveUser: "c", giveSeq: entrySeq, wantUser: "d", wantValid: true},
		{name: "key before every entry lands on the first entry", giveUser: "a", giveSeq: entrySeq, wantUser: "b", wantValid: true},
		{name: "newer version of a stored key lands on the stored one", giveUser: "d", giveSeq: 3, wantUser: "d", wantValid: true},
		{name: "older version of a stored key skips to the next entry", giveUser: "d", giveSeq: 1, wantUser: "f", wantValid: true},
		{name: "key after every entry leaves the iterator invalid", giveUser: "z", giveSeq: entrySeq},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := m.Iter()
			it.SeekGE(internalKey(tt.giveUser, tt.giveSeq, keys.KindSet))
			if it.Valid() != tt.wantValid {
				t.Fatalf("SeekGE(%q@%d) valid = %t, want %t", tt.giveUser, tt.giveSeq, it.Valid(), tt.wantValid)
			}
			if !tt.wantValid {
				return
			}

			wantKey := internalKey(tt.wantUser, entrySeq, keys.KindSet)
			if !bytes.Equal(it.Key(), wantKey) || string(it.Value()) != tt.wantUser {
				t.Errorf("SeekGE(%q@%d) = (%x, %q), want (%x, %q)",
					tt.giveUser, tt.giveSeq, it.Key(), it.Value(), wantKey, tt.wantUser)
			}
		})
	}
}

func TestIterator_SeekGEResolvesVisibleVersion(t *testing.T) {
	t.Parallel()

	m := memtable.New(newFakeRand(19))
	m.Insert(internalKey("k", 1, keys.KindSet), []byte("v1"))
	m.Insert(internalKey("k", 3, keys.KindSet), []byte("v3"))
	m.Insert(internalKey("k", 5, keys.KindDelete), nil)
	m.Insert(internalKey("solo", 2, keys.KindSet), []byte("s2"))
	m.Insert(internalKey("ab", 4, keys.KindSet), []byte("ab4"))

	tests := []struct {
		name      string
		giveUser  string
		giveSeq   keys.Seq
		wantUser  string
		wantSeq   keys.Seq
		wantKind  keys.Kind
		wantValid bool
	}{
		{name: "seek below the oldest version skips past the user key", giveUser: "k", giveSeq: 0, wantUser: "solo", wantSeq: 2, wantKind: keys.KindSet, wantValid: true},
		{name: "oldest version visible at its own seq", giveUser: "k", giveSeq: 1, wantUser: "k", wantSeq: 1, wantKind: keys.KindSet, wantValid: true},
		{name: "oldest version visible between writes", giveUser: "k", giveSeq: 2, wantUser: "k", wantSeq: 1, wantKind: keys.KindSet, wantValid: true},
		{name: "newer version wins at its own seq", giveUser: "k", giveSeq: 3, wantUser: "k", wantSeq: 3, wantKind: keys.KindSet, wantValid: true},
		{name: "newer version wins before the delete", giveUser: "k", giveSeq: 4, wantUser: "k", wantSeq: 3, wantKind: keys.KindSet, wantValid: true},
		{name: "delete visible at its own seq", giveUser: "k", giveSeq: 5, wantUser: "k", wantSeq: 5, wantKind: keys.KindDelete, wantValid: true},
		{name: "delete visible above every version", giveUser: "k", giveSeq: keys.MaxSeq, wantUser: "k", wantSeq: 5, wantKind: keys.KindDelete, wantValid: true},
		{name: "single version key resolves", giveUser: "solo", giveSeq: 2, wantUser: "solo", wantSeq: 2, wantKind: keys.KindSet, wantValid: true},
		{name: "seek below the last stored version leaves the iterator invalid", giveUser: "solo", giveSeq: 1},
		{name: "missing key lands on the next stored key", giveUser: "missing", giveSeq: keys.MaxSeq, wantUser: "solo", wantSeq: 2, wantKind: keys.KindSet, wantValid: true},
		{name: "strict prefix of a stored key lands on that key", giveUser: "a", giveSeq: keys.MaxSeq, wantUser: "ab", wantSeq: 4, wantKind: keys.KindSet, wantValid: true},
		{name: "stored key matches in full", giveUser: "ab", giveSeq: keys.MaxSeq, wantUser: "ab", wantSeq: 4, wantKind: keys.KindSet, wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := m.Iter()
			it.SeekGE(keys.AppendSeek(nil, []byte(tt.giveUser), tt.giveSeq))
			if it.Valid() != tt.wantValid {
				t.Fatalf("SeekGE(seek %q@%d) valid = %t, want %t", tt.giveUser, tt.giveSeq, it.Valid(), tt.wantValid)
			}
			if !tt.wantValid {
				return
			}

			user := string(keys.UserKey(it.Key()))
			seq, kind := keys.Trailer(it.Key())
			if user != tt.wantUser || seq != tt.wantSeq || kind != tt.wantKind {
				t.Errorf("SeekGE(seek %q@%d) = (%q, %d, %d), want (%q, %d, %d)",
					tt.giveUser, tt.giveSeq, user, seq, kind, tt.wantUser, tt.wantSeq, tt.wantKind)
			}
		})
	}
}

func TestIterator_FirstPositionsAtSmallestEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		giveUsers []string
		wantUser  string
		wantValid bool
	}{
		{name: "empty memtable has no first entry"},
		{name: "single entry memtable lands on that entry", giveUsers: []string{"only"}, wantUser: "only", wantValid: true},
		{name: "unordered inserts land on the smallest user key", giveUsers: []string{"m", "a", "z"}, wantUser: "a", wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := memtable.New(newFakeRand(23))
			for _, user := range tt.giveUsers {
				m.Insert(internalKey(user, 1, keys.KindSet), []byte(user))
			}

			it := m.Iter()
			it.First()
			if it.Valid() != tt.wantValid {
				t.Fatalf("First() valid = %t, want %t", it.Valid(), tt.wantValid)
			}
			if !tt.wantValid {
				return
			}

			user := string(keys.UserKey(it.Key()))
			if user != tt.wantUser || string(it.Value()) != tt.wantUser {
				t.Errorf("First() = (%q, %q), want (%q, %q)", user, it.Value(), tt.wantUser, tt.wantUser)
			}
		})
	}
}

func TestIterator_WalkMatchesAll(t *testing.T) {
	t.Parallel()

	const entries = 200

	m := memtable.New(newFakeRand(29))
	for step := range entries {
		index := (step * 37) % entries
		m.Insert(keys.Append(nil, numberedUser(index), keys.Seq(index+1), keys.KindSet), []byte(numberedValue(index)))
	}

	wantKeys, wantValues := collectAll(m)
	gotKeys, gotValues := collectIter(m.Iter())
	if !slices.EqualFunc(gotKeys, wantKeys, bytes.Equal) || !slices.EqualFunc(gotValues, wantValues, bytes.Equal) {
		t.Errorf("iterator walk = (%x, %q), want (%x, %q)", gotKeys, gotValues, wantKeys, wantValues)
	}
	if len(gotKeys) != entries {
		t.Errorf("iterator walk yielded %d entries, want %d", len(gotKeys), entries)
	}
}

func TestIterator_WalksWhileWriterInserts(t *testing.T) {
	t.Parallel()

	const (
		entries = 2000
		walkers = 4
	)

	want := make(map[string]string, entries)
	for i := range entries {
		want[string(keys.Append(nil, numberedUser(i), keys.Seq(i+1), keys.KindSet))] = numberedValue(i)
	}

	m := memtable.New(newFakeRand(31))
	done := make(chan struct{})

	var wg sync.WaitGroup
	for range walkers {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}

				var previous []byte
				it := m.Iter()
				for it.First(); it.Valid(); it.Next() {
					ikey, value := it.Key(), it.Value()
					if got, ok := want[string(ikey)]; !ok || got != string(value) {
						t.Errorf("walk yielded (%x, %q), want value %q", ikey, value, got)
					}
					if previous != nil && keys.Compare(previous, ikey) >= 0 {
						t.Errorf("walk yielded %x after %x, want strictly ascending keys", ikey, previous)
					}
					previous = ikey
				}
			}
		})
	}

	for step := range entries {
		index := (step * 37) % entries
		m.Insert(keys.Append(nil, numberedUser(index), keys.Seq(index+1), keys.KindSet), []byte(numberedValue(index)))
	}
	close(done)
	wg.Wait()

	gotKeys, _ := collectIter(m.Iter())
	if len(gotKeys) != entries {
		t.Errorf("walk after all inserts yielded %d entries, want %d", len(gotKeys), entries)
	}
	if !slices.IsSortedFunc(gotKeys, keys.Compare) {
		t.Errorf("walk after all inserts yielded %x, want comparator order", gotKeys)
	}
}

func TestIterator_PositionedOperationsPanicWhenInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		giveUsers []string
		giveSeek  string
	}{
		{name: "empty memtable has nothing to position on", giveSeek: "a"},
		{name: "seek past the last entry has nothing to position on", giveUsers: []string{"a", "b"}, giveSeek: "z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := memtable.New(newFakeRand(37))
			for _, user := range tt.giveUsers {
				m.Insert(internalKey(user, 1, keys.KindSet), []byte(user))
			}

			it := m.Iter()
			it.SeekGE(internalKey(tt.giveSeek, 1, keys.KindSet))
			if it.Valid() {
				t.Fatalf("SeekGE(%q@1) valid = true, want false", tt.giveSeek)
			}

			assertPanics(t, "Key()", func() { it.Key() })
			assertPanics(t, "Value()", func() { it.Value() })
			assertPanics(t, "Next()", func() { it.Next() })
		})
	}
}
