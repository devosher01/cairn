package memtable_test

import (
	"sync"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/memtable"
)

func TestMemtable_ReadersStayConsistentDuringInserts(t *testing.T) {
	t.Parallel()

	const (
		entries = 5000
		readers = 4
	)

	want := make(map[string]string, entries)
	for i := range entries {
		want[string(keys.Append(nil, numberedUser(i), keys.Seq(i+1), keys.KindSet))] = numberedValue(i)
	}

	m := memtable.New(newFakeRand(7))
	done := make(chan struct{})

	var wg sync.WaitGroup
	for reader := range readers {
		wg.Go(func() {
			rnd := newFakeRand(uint64(reader) + 1)
			seen := 0
			for {
				select {
				case <-done:
					return
				default:
				}

				index := int(rnd.Uint64() % entries)
				if value, kind, ok := m.Get(numberedUser(index), keys.MaxSeq); ok {
					if kind != keys.KindSet || string(value) != numberedValue(index) {
						t.Errorf("Get(%q) = (%q, %d), want (%q, %d)",
							numberedUser(index), value, kind, numberedValue(index), keys.KindSet)
					}
				}

				var previous []byte
				count := 0
				for ikey, value := range m.All() {
					if previous != nil && keys.Compare(previous, ikey) >= 0 {
						t.Errorf("All yielded %x after %x, want ascending order", ikey, previous)
					}
					if got, ok := want[string(ikey)]; !ok || got != string(value) {
						t.Errorf("All yielded (%x, %q), want value %q", ikey, value, got)
					}
					previous = ikey
					count++
				}
				if count < seen {
					t.Errorf("All yielded %d entries after having yielded %d, want no entry to disappear", count, seen)
				}
				seen = count
			}
		})
	}

	for step := range entries {
		index := (step * 37) % entries
		m.Insert(keys.Append(nil, numberedUser(index), keys.Seq(index+1), keys.KindSet), []byte(numberedValue(index)))
	}
	close(done)
	wg.Wait()

	if got := m.Len(); got != entries {
		t.Errorf("Len() = %d, want %d", got, entries)
	}
	if ikeys, _ := collectAll(m); len(ikeys) != entries {
		t.Errorf("All yielded %d entries, want %d", len(ikeys), entries)
	}
}
