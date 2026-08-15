package sstable

import (
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
)

func TestTable_AllYieldsEveryEntryInWrittenOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		giveOpt WriterOptions
		give    []tableEntry
	}{
		{name: "single entry", give: []tableEntry{{user: "only", seq: 1, kind: keys.KindSet, value: "v"}}},
		{
			name: "empty values",
			give: []tableEntry{
				{user: "a", seq: 3, kind: keys.KindSet},
				{user: "b", seq: 2, kind: keys.KindDelete},
				{user: "c", seq: 1, kind: keys.KindSet, value: "c"},
			},
		},
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

			if got := dumpTable(t, table); !slices.Equal(got, tt.give) {
				t.Errorf("All yielded %+v, want %+v", got, tt.give)
			}
		})
	}
}

func TestTable_GetResolvesSnapshotVisibility(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{}, versionedEntries())
	table := openTable(t, data)

	tests := []struct {
		name      string
		giveUser  string
		giveSeq   keys.Seq
		wantValue string
		wantKind  keys.Kind
		wantFound bool
	}{
		{name: "snapshot below every version", giveUser: "pear", giveSeq: 9},
		{
			name: "snapshot at the oldest version", giveUser: "pear", giveSeq: 10,
			wantValue: "pear v10", wantKind: keys.KindSet, wantFound: true,
		},
		{
			name: "snapshot between versions", giveUser: "pear", giveSeq: 15,
			wantValue: "pear v10", wantKind: keys.KindSet, wantFound: true,
		},
		{
			name: "snapshot at the tombstone", giveUser: "pear", giveSeq: 20,
			wantKind: keys.KindDelete, wantFound: true,
		},
		{
			name: "snapshot above the tombstone", giveUser: "pear", giveSeq: 25,
			wantKind: keys.KindDelete, wantFound: true,
		},
		{
			name: "snapshot at the newest version", giveUser: "pear", giveSeq: 30,
			wantValue: "pear v30", wantKind: keys.KindSet, wantFound: true,
		},
		{
			name: "the newest sequence sees the newest version", giveUser: "pear", giveSeq: keys.MaxSeq,
			wantValue: "pear v30", wantKind: keys.KindSet, wantFound: true,
		},
		{
			name: "first user key of the table", giveUser: "apple", giveSeq: keys.MaxSeq,
			wantValue: "apple v4", wantKind: keys.KindSet, wantFound: true,
		},
		{
			name: "last user key of the table", giveUser: "plum", giveSeq: keys.MaxSeq,
			wantValue: "plum v7", wantKind: keys.KindSet, wantFound: true,
		},
		{name: "first user key below its sequence", giveUser: "apple", giveSeq: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, kind, ok, err := table.Get([]byte(tt.giveUser), tt.giveSeq)
			if err != nil {
				t.Fatalf("Get(%q, %d): %v", tt.giveUser, tt.giveSeq, err)
			}
			if ok != tt.wantFound || kind != tt.wantKind || string(value) != tt.wantValue {
				t.Errorf("Get(%q, %d) = (%q, %d, %t), want (%q, %d, %t)",
					tt.giveUser, tt.giveSeq, value, kind, ok, tt.wantValue, tt.wantKind, tt.wantFound)
			}
		})
	}
}

func TestTable_GetMissesAbsentUserKeys(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	table := openTable(t, data)

	tests := []struct {
		name string
		give string
	}{
		{name: "before the first key", give: "aaa"},
		{name: "after the last key", give: "zzz"},
		{name: "between two keys", give: "cobalt"},
		{name: "prefix of a stored key", give: "alph"},
		{name: "stored key with a suffix", give: "alphax"},
		{name: "empty user key", give: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, kind, ok, err := table.Get([]byte(tt.give), keys.MaxSeq)
			if err != nil {
				t.Fatalf("Get(%q): %v", tt.give, err)
			}
			if ok {
				t.Errorf("Get(%q) = (%q, %d, true), want not found", tt.give, value, kind)
			}
		})
	}
}

func TestTable_GetRejectsBloomPositiveAbsentKeys(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 128, BloomBitsPerKey: 1}, manyEntries(200))
	table := openTable(t, data)

	tests := []struct {
		name string
		give string
	}{
		{name: "sorting before the first block", give: "absent"},
		{name: "sorting inside a block", give: "key0100x"},
		{name: "sorting past the last block", give: "zzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			probe := bloomPositiveKey(t, table, tt.give)

			value, kind, ok, err := table.Get([]byte(probe), keys.MaxSeq)
			if err != nil {
				t.Fatalf("Get(%q): %v", probe, err)
			}
			if ok {
				t.Errorf("Get(%q) = (%q, %d, true), want not found", probe, value, kind)
			}
		})
	}
}

func TestTable_GetFindsKeysOnBothSidesOfEveryBlockBoundary(t *testing.T) {
	t.Parallel()

	entries := manyEntries(60)
	data, _ := buildTable(t, WriterOptions{BlockSize: 128}, entries)
	table := openTable(t, data)

	if got := len(table.index); got != 30 {
		t.Fatalf("table holds %d blocks, want 30", got)
	}

	for _, e := range entries {
		value, kind, ok, err := table.Get([]byte(e.user), e.seq)
		if err != nil {
			t.Fatalf("Get(%q, %d): %v", e.user, e.seq, err)
		}
		if !ok || kind != e.kind || string(value) != e.value {
			t.Errorf("Get(%q, %d) = (%q, %d, %t), want (%q, %d, true)",
				e.user, e.seq, value, kind, ok, e.value, e.kind)
		}
	}

	for i, e := range table.index {
		if got := string(keys.UserKey(e.lastKey)); got != entries[2*i+1].user {
			t.Errorf("block %d ends at %q, want %q", i, got, entries[2*i+1].user)
		}
	}
}

func TestTable_GetRejectsUnknownKinds(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{}, []tableEntry{{user: "key", seq: 1, kind: keys.Kind(0x7f), value: "v"}})
	table := openTable(t, data)

	if _, _, _, err := table.Get([]byte("key"), keys.MaxSeq); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Get error = %v, want %v", err, ErrCorrupt)
	}
}

func TestOpen_RejectsCorruptTables(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	ft := tableFooter(t, data)
	limit := uint64(len(data) - _footerSize)

	base := openTable(t, data)
	first, second := base.index[0], base.index[1]
	goodHandle := appendHandle(nil, uint64(first.offset), uint64(first.length))

	tests := []struct {
		name string
		give []byte
	}{
		{name: "empty file", give: nil},
		{name: "shorter than the footer", give: data[:_footerSize-1]},
		{name: "zeroed footer sized file", give: make([]byte, _footerSize)},
		{name: "truncated into the footer", give: data[:len(data)-10]},
		{name: "truncated into the index block", give: data[:int(ft.indexOffset)+1]},
		{name: "flipped footer magic", give: flipBlockByte(data, len(data)-1)},
		{name: "flipped footer crc", give: flipBlockByte(data, len(data)-_footerSize+_footerCRCAt)},
		{name: "unknown footer version", give: setFooterVersion(data, _footerVersion+1)},
		{name: "flipped index block byte", give: flipBlockByte(data, int(ft.indexOffset))},
		{name: "flipped filter block byte", give: flipBlockByte(data, int(ft.filterOffset))},
		{
			name: "index offset past the data region",
			give: rewriteFooter(data, footer{
				indexOffset: limit, indexLength: ft.indexLength,
				filterOffset: ft.filterOffset, filterLength: ft.filterLength,
			}),
		},
		{
			name: "index length past the end of the file",
			give: rewriteFooter(data, footer{
				indexOffset: ft.indexOffset, indexLength: limit + 1,
				filterOffset: ft.filterOffset, filterLength: ft.filterLength,
			}),
		},
		{
			name: "index length below the block trailer size",
			give: rewriteFooter(data, footer{
				indexOffset: ft.indexOffset, indexLength: _blockTrailerSize - 1,
				filterOffset: ft.filterOffset, filterLength: ft.filterLength,
			}),
		},
		{
			name: "filter block overlapping the footer",
			give: rewriteFooter(data, footer{
				indexOffset: ft.indexOffset, indexLength: ft.indexLength,
				filterOffset: limit - ft.filterLength + 1, filterLength: ft.filterLength,
			}),
		},
		{name: "index block without entries", give: withCraftedIndex(t, data, nil)},
		{name: "index block with a truncated entry", give: withCraftedIndexPayload(t, data, []byte{0x09, 0x80})},
		{
			name: "index entry key shorter than the trailer",
			give: withCraftedIndex(t, data, []craftedIndexEntry{{lastKey: []byte("short"), handle: goodHandle}}),
		},
		{
			name: "index entry handle offset past the data region",
			give: withCraftedIndex(t, data, []craftedIndexEntry{
				{lastKey: first.lastKey, handle: appendHandle(nil, 1<<40, uint64(first.length))},
			}),
		},
		{
			name: "index entry handle length below the block trailer size",
			give: withCraftedIndex(t, data, []craftedIndexEntry{
				{lastKey: first.lastKey, handle: appendHandle(nil, 0, _blockTrailerSize-1)},
			}),
		},
		{
			name: "index entry handle with a truncated uvarint",
			give: withCraftedIndex(t, data, []craftedIndexEntry{{lastKey: first.lastKey, handle: []byte{0x80}}}),
		},
		{
			name: "index entry handle missing its length",
			give: withCraftedIndex(t, data, []craftedIndexEntry{{lastKey: first.lastKey, handle: []byte{0x01}}}),
		},
		{
			name: "index entry handle with trailing bytes",
			give: withCraftedIndex(t, data, []craftedIndexEntry{
				{lastKey: first.lastKey, handle: append(slices.Clone(goodHandle), 0x00)},
			}),
		},
		{
			name: "index entries out of order",
			give: withCraftedIndex(t, data, []craftedIndexEntry{
				{lastKey: second.lastKey, handle: goodHandle},
				{lastKey: first.lastKey, handle: goodHandle},
			}),
		},
		{
			name: "repeated index entry key",
			give: withCraftedIndex(t, data, []craftedIndexEntry{
				{lastKey: first.lastKey, handle: goodHandle},
				{lastKey: first.lastKey, handle: goodHandle},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := Open(&fakeFile{data: tt.give}, int64(len(tt.give)))
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Open error = %v, want %v", err, ErrCorrupt)
			}
			if table != nil {
				t.Errorf("Open returned a table alongside the error")
			}
		})
	}
}

func TestOpen_RejectsFilesShorterThanTheirDeclaredSize(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())

	tests := []struct {
		name string
		give int
	}{
		{name: "cut inside the footer", give: len(data) - 4},
		{name: "cut inside the index block", give: int(tableFooter(t, data).indexOffset) + 2},
		{name: "cut inside the first data block", give: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := Open(&fakeFile{data: data[:tt.give]}, int64(len(data)))
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Open error = %v, want %v", err, ErrCorrupt)
			}
			if table != nil {
				t.Errorf("Open returned a table alongside the error")
			}
		})
	}
}

func TestTable_ReportsCorruptDataBlocksOnlyForTheBlocksThatHoldThem(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	table := openTable(t, flipBlockByte(data, 3))

	if _, _, _, err := table.Get([]byte("alpha"), keys.MaxSeq); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Get of a key in the damaged block: error = %v, want %v", err, ErrCorrupt)
	}

	value, kind, ok, err := table.Get([]byte("omega"), keys.MaxSeq)
	if err != nil {
		t.Fatalf("Get of a key in an intact block: %v", err)
	}
	if !ok || kind != keys.KindSet || string(value) != "value for omega at 1" {
		t.Errorf("Get(omega) = (%q, %d, %t), want (%q, %d, true)",
			value, kind, ok, "value for omega at 1", keys.KindSet)
	}

	var yielded int
	for range table.All() {
		yielded++
	}
	if err := table.AllErr(); !errors.Is(err, ErrCorrupt) {
		t.Errorf("AllErr = %v, want %v", err, ErrCorrupt)
	}
	if yielded != 0 {
		t.Errorf("All yielded %d entries from a damaged first block, want 0", yielded)
	}
}

func TestTable_AllStopsAtTheDamagedBlockAndKeepsTheEarlierEntries(t *testing.T) {
	t.Parallel()

	entries := goldenEntries()
	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, entries)
	table := openTable(t, data)

	damaged := table.index[len(table.index)-1]
	broken := openTable(t, flipBlockByte(data, int(damaged.offset)))

	var got []tableEntry
	for ikey, value := range broken.All() {
		seq, kind := keys.Trailer(ikey)
		got = append(got, tableEntry{user: string(keys.UserKey(ikey)), seq: seq, kind: kind, value: string(value)})
	}

	if err := broken.AllErr(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("AllErr = %v, want %v", err, ErrCorrupt)
	}
	if len(got) == 0 || len(got) >= len(entries) {
		t.Fatalf("All yielded %d entries, want a shorter non-empty prefix of the %d written", len(got), len(entries))
	}
	if want := entries[:len(got)]; !slices.Equal(got, want) {
		t.Errorf("All yielded %+v, want %+v", got, want)
	}
	if err := table.AllErr(); err != nil {
		t.Errorf("AllErr on the intact table = %v, want nil", err)
	}
}

func TestTable_AllClearsTheErrorOfAnEarlierWalk(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())

	f := failingReadFile(data, 3)
	table, err := Open(f, int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for range table.All() {
	}
	if err := table.AllErr(); !errors.Is(err, errRead) {
		t.Fatalf("AllErr = %v, want %v", err, errRead)
	}

	f.readErr = nil
	if got := dumpTable(t, table); !slices.Equal(got, goldenEntries()) {
		t.Errorf("All yielded %+v, want %+v", got, goldenEntries())
	}
}

func TestTable_RejectsDataBlocksHoldingUnreadableEntries(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())
	ft := tableFooter(t, data)
	high := tableEntry{user: "zzzz", seq: 1, kind: keys.KindSet}.ikey()

	table := openTable(t, withCraftedIndex(t, data, []craftedIndexEntry{
		{lastKey: high, handle: appendHandle(nil, ft.filterOffset, ft.filterLength)},
	}))

	if _, _, _, err := table.Get([]byte("alpha"), keys.MaxSeq); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Get error = %v, want %v", err, ErrCorrupt)
	}

	var yielded int
	for range table.All() {
		yielded++
	}
	if err := table.AllErr(); !errors.Is(err, ErrCorrupt) {
		t.Errorf("AllErr = %v, want %v", err, ErrCorrupt)
	}
	if yielded != 0 {
		t.Errorf("All yielded %d entries from an unreadable block, want 0", yielded)
	}
}

func TestTable_RejectsBlockEntriesWithKeysShorterThanTheTrailer(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64, BloomBitsPerKey: 1}, goldenEntries())
	high := tableEntry{user: "zzzzzzzz", seq: 1, kind: keys.KindSet}.ikey()
	payload := buildBlockPayload([]blockEntry{{ikey: "abcd", value: "v"}})

	table := openTable(t, withCraftedBlock(t, data, payload, high))
	probe := bloomPositiveKey(t, table, "zzz")

	if _, _, _, err := table.Get([]byte(probe), keys.MaxSeq); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Get error = %v, want %v", err, ErrCorrupt)
	}

	var yielded int
	for range table.All() {
		yielded++
	}
	if err := table.AllErr(); !errors.Is(err, ErrCorrupt) {
		t.Errorf("AllErr = %v, want %v", err, ErrCorrupt)
	}
	if yielded != 0 {
		t.Errorf("All yielded %d entries from a block holding a short key, want 0", yielded)
	}
}

func TestTable_GetMissesWhenTheIndexOverstatesItsBlock(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64, BloomBitsPerKey: 1}, goldenEntries())
	first := openTable(t, data).index[0]
	high := tableEntry{user: "zzzzzzzz", seq: 1, kind: keys.KindSet}.ikey()

	table := openTable(t, withCraftedIndex(t, data, []craftedIndexEntry{
		{lastKey: high, handle: appendHandle(nil, uint64(first.offset), uint64(first.length))},
	}))
	probe := bloomPositiveKey(t, table, "zzz")

	value, kind, ok, err := table.Get([]byte(probe), keys.MaxSeq)
	if err != nil {
		t.Fatalf("Get(%q): %v", probe, err)
	}
	if ok {
		t.Errorf("Get(%q) = (%q, %d, true), want not found", probe, value, kind)
	}
}

func TestTable_AllStopsWhenTheConsumerBreaks(t *testing.T) {
	t.Parallel()

	entries := goldenEntries()
	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, entries)
	table := openTable(t, data)

	var got []tableEntry
	for ikey, value := range table.All() {
		seq, kind := keys.Trailer(ikey)
		got = append(got, tableEntry{user: string(keys.UserKey(ikey)), seq: seq, kind: kind, value: string(value)})
		if len(got) == 2 {
			break
		}
	}

	if want := entries[:2]; !slices.Equal(got, want) {
		t.Errorf("All yielded %+v before the break, want %+v", got, want)
	}
	if err := table.AllErr(); err != nil {
		t.Errorf("AllErr = %v, want nil", err)
	}
}

func TestTable_GetPropagatesFileReadErrors(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())

	table, err := Open(failingReadFile(data, 3), int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, _, _, err = table.Get([]byte("alpha"), keys.MaxSeq)
	if !errors.Is(err, errRead) {
		t.Fatalf("Get error = %v, want %v", err, errRead)
	}
	if errors.Is(err, ErrCorrupt) {
		t.Errorf("Get error = %v, want a read failure rather than corruption", err)
	}
}

func TestOpen_PropagatesFileReadErrors(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{BlockSize: 64}, goldenEntries())

	tests := []struct {
		name      string
		giveAfter int64
	}{
		{name: "footer", giveAfter: 0},
		{name: "index block", giveAfter: 1},
		{name: "filter block", giveAfter: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := Open(failingReadFile(data, tt.giveAfter), int64(len(data)))
			if !errors.Is(err, errRead) {
				t.Fatalf("Open error = %v, want %v", err, errRead)
			}
			if table != nil {
				t.Errorf("Open returned a table alongside the error")
			}
		})
	}
}

func TestTable_CloseClosesTheFile(t *testing.T) {
	t.Parallel()

	data, _ := buildTable(t, WriterOptions{}, goldenEntries())

	f := &fakeFile{data: data}
	table, err := Open(f, int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := table.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if f.closes != 1 {
		t.Errorf("file closed %d times, want 1", f.closes)
	}

	failing := &fakeFile{data: data, closeErr: errClose}
	table, err = Open(failing, int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := table.Close(); !errors.Is(err, errClose) {
		t.Errorf("Close error = %v, want %v", err, errClose)
	}
}
