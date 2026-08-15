package sstable

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
)

func iterEntry(it *TableIter) tableEntry {
	seq, kind := keys.Trailer(it.Key())

	return tableEntry{
		user:  string(keys.UserKey(it.Key())),
		seq:   seq,
		kind:  kind,
		value: string(it.Value()),
	}
}

func walkIter(it *TableIter) []tableEntry {
	var got []tableEntry
	for it.First(); it.Valid(); it.Next() {
		got = append(got, iterEntry(it))
	}

	return got
}

func drainIter(it *TableIter) []tableEntry {
	var got []tableEntry
	for ; it.Valid(); it.Next() {
		got = append(got, iterEntry(it))
	}

	return got
}

func TestTableIter_WalksEveryEntryLikeAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		giveOpt WriterOptions
		give    []tableEntry
	}{
		{name: "single entry", give: []tableEntry{{user: "only", seq: 1, kind: keys.KindSet, value: "v"}}},
		{name: "versions of one user key including a tombstone", give: versionedEntries()},
		{name: "golden entries over tiny blocks", giveOpt: WriterOptions{BlockSize: 64}, give: goldenEntries()},
		{name: "one hundred entries over several default sized blocks", give: manyEntries(100)},
		{
			name:    "one hundred entries over one block each",
			giveOpt: WriterOptions{BlockSize: 1},
			give:    manyEntries(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, _ := buildTable(t, tt.giveOpt, tt.give)
			table := openTable(t, data)

			it := table.Iter()
			got := walkIter(it)
			if err := it.Err(); err != nil {
				t.Fatalf("Err after a full walk = %v, want nil", err)
			}
			if want := dumpTable(t, table); !slices.Equal(got, want) {
				t.Errorf("the walk yielded %+v, want %+v", got, want)
			}
		})
	}
}

func TestTableIter_SeekGELandsOnTheFirstEntryAtOrAfterTheTarget(t *testing.T) {
	t.Parallel()

	entries := manyEntries(60)
	data, _ := buildTable(t, WriterOptions{BlockSize: 128}, entries)
	table := openTable(t, data)

	if got := len(table.index); got != 30 {
		t.Fatalf("table holds %d blocks, want 30", got)
	}

	tests := []struct {
		name      string
		give      []byte
		wantAt    int
		wantValid bool
	}{
		{name: "the exact first key", give: entries[0].ikey(), wantAt: 0, wantValid: true},
		{name: "the exact last key", give: entries[59].ikey(), wantAt: 59, wantValid: true},
		{
			name: "a key sorting before every entry",
			give: keys.Append(nil, []byte("aaa"), 1, keys.KindSet), wantAt: 0, wantValid: true,
		},
		{name: "the last key of a block", give: entries[1].ikey(), wantAt: 1, wantValid: true},
		{
			name: "a key between two blocks",
			give: keys.Append(nil, []byte("key0001x"), 1, keys.KindSet), wantAt: 2, wantValid: true,
		},
		{
			name: "a key between two entries of one block",
			give: keys.Append(nil, []byte("key0002x"), 1, keys.KindSet), wantAt: 3, wantValid: true,
		},
		{name: "a key sorting past every entry", give: keys.Append(nil, []byte("zzz"), 1, keys.KindSet)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := table.Iter()
			it.SeekGE(tt.give)

			if err := it.Err(); err != nil {
				t.Fatalf("Err after SeekGE = %v, want nil", err)
			}
			if it.Valid() != tt.wantValid {
				t.Fatalf("Valid after SeekGE = %t, want %t", it.Valid(), tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if got := iterEntry(it); got != entries[tt.wantAt] {
				t.Fatalf("SeekGE landed on %+v, want %+v", got, entries[tt.wantAt])
			}
			if got, want := drainIter(it), entries[tt.wantAt:]; !slices.Equal(got, want) {
				t.Errorf("the walk from the landing yielded %+v, want %+v", got, want)
			}
		})
	}
}

func TestTableIter_SeekGELandsOnTheNewestVersionVisibleToTheSeek(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, versionedEntries())
	table := openTable(t, data)

	tests := []struct {
		name     string
		giveUser string
		giveSeq  keys.Seq
		want     tableEntry
	}{
		{
			name: "the newest sequence sees the newest version", giveUser: "pear", giveSeq: keys.MaxSeq,
			want: tableEntry{user: "pear", seq: 30, kind: keys.KindSet, value: "pear v30"},
		},
		{
			name: "a seek at the newest version", giveUser: "pear", giveSeq: 30,
			want: tableEntry{user: "pear", seq: 30, kind: keys.KindSet, value: "pear v30"},
		},
		{
			name: "a seek above the tombstone", giveUser: "pear", giveSeq: 25,
			want: tableEntry{user: "pear", seq: 20, kind: keys.KindDelete},
		},
		{
			name: "a seek at the tombstone", giveUser: "pear", giveSeq: 20,
			want: tableEntry{user: "pear", seq: 20, kind: keys.KindDelete},
		},
		{
			name: "a seek between the tombstone and the oldest version", giveUser: "pear", giveSeq: 15,
			want: tableEntry{user: "pear", seq: 10, kind: keys.KindSet, value: "pear v10"},
		},
		{
			name: "a seek below every version steps to the next user key", giveUser: "pear", giveSeq: 9,
			want: tableEntry{user: "plum", seq: 7, kind: keys.KindSet, value: "plum v7"},
		},
		{
			name: "a seek below the first user key of the table", giveUser: "apple", giveSeq: 3,
			want: tableEntry{user: "pear", seq: 30, kind: keys.KindSet, value: "pear v30"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := table.Iter()
			it.SeekGE(keys.AppendSeek(nil, []byte(tt.giveUser), tt.giveSeq))

			if err := it.Err(); err != nil {
				t.Fatalf("Err after SeekGE = %v, want nil", err)
			}
			if !it.Valid() {
				t.Fatalf("SeekGE(%q, %d) left the iterator invalid", tt.giveUser, tt.giveSeq)
			}
			if got := iterEntry(it); got != tt.want {
				t.Errorf("SeekGE(%q, %d) landed on %+v, want %+v", tt.giveUser, tt.giveSeq, got, tt.want)
			}
		})
	}
}

func TestTableIter_SeekGEWalksPastBlocksTheIndexOverstates(t *testing.T) {
	t.Parallel()

	entries := manyEntries(60)
	data, _ := buildTable(t, WriterOptions{BlockSize: 128}, entries)
	index := openTable(t, data).index

	overstated := craftedIndexEntry{
		lastKey: keys.Append(nil, []byte("key0001x"), 1, keys.KindSet),
		handle:  appendHandle(nil, uint64(index[0].offset), uint64(index[0].length)),
	}
	second := craftedIndexEntry{
		lastKey: index[1].lastKey,
		handle:  appendHandle(nil, uint64(index[1].offset), uint64(index[1].length)),
	}

	tests := []struct {
		name      string
		give      []byte
		wantAt    int
		wantValid bool
	}{
		{
			name:   "the scan crosses into the next block",
			give:   withCraftedIndex(t, data, []craftedIndexEntry{overstated, second}),
			wantAt: 2, wantValid: true,
		},
		{
			name: "the scan runs out of blocks",
			give: withCraftedIndex(t, data, []craftedIndexEntry{overstated}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := openTable(t, tt.give).Iter()
			it.SeekGE(keys.Append(nil, []byte("key0001w"), 1, keys.KindSet))

			if err := it.Err(); err != nil {
				t.Fatalf("Err after SeekGE = %v, want nil", err)
			}
			if it.Valid() != tt.wantValid {
				t.Fatalf("Valid after SeekGE = %t, want %t", it.Valid(), tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if got := iterEntry(it); got != entries[tt.wantAt] {
				t.Errorf("SeekGE landed on %+v, want %+v", got, entries[tt.wantAt])
			}
		})
	}
}

func TestTableIter_StopsAtTheDamagedBlockAndSeeksAgainAfterIt(t *testing.T) {
	t.Parallel()

	entries := goldenEntries()
	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, entries)
	intact := openTable(t, data)
	if got := len(intact.index); got < 3 {
		t.Fatalf("table holds %d blocks, want at least 3", got)
	}

	var head []tableEntry
	for _, e := range entries {
		if keys.Compare(e.ikey(), intact.index[0].lastKey) > 0 {
			break
		}
		head = append(head, e)
	}

	table := openTable(t, flipBlockByte(data, int(intact.index[1].offset)))
	it := table.Iter()

	if got := walkIter(it); !slices.Equal(got, head) {
		t.Errorf("the walk into the damaged block yielded %+v, want %+v", got, head)
	}
	if err := it.Err(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Err after the walk = %v, want %v", err, ErrCorrupt)
	}

	it.Next()
	if err := it.Err(); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Err after Next on the failed iterator = %v, want %v", err, ErrCorrupt)
	}

	it.SeekGE(entries[len(head)].ikey())
	if it.Valid() {
		t.Errorf("SeekGE into the damaged block left the iterator on %+v, want invalid", iterEntry(it))
	}
	if err := it.Err(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Err after SeekGE into the damaged block = %v, want %v", err, ErrCorrupt)
	}

	it.SeekGE(entries[0].ikey())
	if err := it.Err(); err != nil {
		t.Fatalf("Err after SeekGE into an intact block = %v, want nil", err)
	}
	if got := drainIter(it); !slices.Equal(got, head) {
		t.Errorf("the walk after the re-seek yielded %+v, want %+v", got, head)
	}
}

func TestTableIter_RejectsEntriesTheBlockCannotDescribe(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	high := tableEntry{user: "zzzzzzzz", seq: 1, kind: keys.KindSet}.ikey()
	unknownKind := tableEntry{user: "key", seq: 1, kind: keys.Kind(0x7f)}.ikey()

	tests := []struct {
		name string
		give []byte
	}{
		{name: "a key shorter than the trailer", give: buildBlockPayload([]blockEntry{{ikey: "abcd", value: "v"}})},
		{
			name: "an unknown kind",
			give: buildBlockPayload([]blockEntry{{ikey: string(unknownKind), value: "v"}}),
		},
		{name: "a truncated entry", give: []byte{0x09, 0x80}},
		{name: "a key overrunning the payload", give: []byte{0x09, 0x00, 'k'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table := openTable(t, withCraftedBlock(t, data, tt.give, high))

			it := table.Iter()
			it.First()
			if it.Valid() {
				t.Errorf("First left the iterator on %+v, want invalid", iterEntry(it))
			}
			if err := it.Err(); !errors.Is(err, ErrCorrupt) {
				t.Errorf("Err after First = %v, want %v", err, ErrCorrupt)
			}
		})
	}
}

func TestTableIter_PropagatesFileReadErrors(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())

	f := failingReadFile(data, 3)
	table, err := Open(f, int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	it := table.Iter()
	it.First()
	if it.Valid() {
		t.Errorf("First left the iterator on %+v, want invalid", iterEntry(it))
	}
	if err := it.Err(); !errors.Is(err, errRead) {
		t.Fatalf("Err after First = %v, want %v", err, errRead)
	}
	if errors.Is(it.Err(), ErrCorrupt) {
		t.Errorf("Err = %v, want a read failure rather than corruption", it.Err())
	}

	f.readErr = nil
	if got := walkIter(it); !slices.Equal(got, goldenEntries()) {
		t.Errorf("the walk after the read failure cleared yielded %+v, want %+v", got, goldenEntries())
	}
	if err := it.Err(); err != nil {
		t.Errorf("Err after the second walk = %v, want nil", err)
	}
}

func TestTableIter_PanicsOnAccessWhileInvalid(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	table := openTable(t, data)

	fresh := table.Iter()
	wantPanic(t, "Key on a fresh iterator", func() { fresh.Key() })
	wantPanic(t, "Value on a fresh iterator", func() { fresh.Value() })

	exhausted := table.Iter()
	walkIter(exhausted)
	wantPanic(t, "Key on an exhausted iterator", func() { exhausted.Key() })
	wantPanic(t, "Value on an exhausted iterator", func() { exhausted.Value() })
}

func TestTableIter_IteratorsOnOneTableAreIndependent(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 128}, manyEntries(100))
	table := openTable(t, data)
	want := dumpTable(t, table)

	walks := make([][]tableEntry, 2)
	fails := make([]error, 2)

	var wg sync.WaitGroup
	for at := range walks {
		wg.Go(func() {
			it := table.Iter()
			walks[at] = walkIter(it)
			fails[at] = it.Err()
		})
	}
	wg.Wait()

	for at, got := range walks {
		if fails[at] != nil {
			t.Errorf("iterator %d: Err = %v, want nil", at, fails[at])
		}
		if !slices.Equal(got, want) {
			t.Errorf("iterator %d yielded %+v, want %+v", at, got, want)
		}
	}
}
