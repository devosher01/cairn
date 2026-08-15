package sstable

import (
	"bytes"
	"errors"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
)

func TestWriter_ReportsMetaForTheWrittenTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		giveOpt WriterOptions
		give    []tableEntry
	}{
		{name: "single entry", give: []tableEntry{{user: "only", seq: 1, kind: keys.KindSet, value: "v"}}},
		{name: "single tombstone", give: []tableEntry{{user: "only", seq: 1, kind: keys.KindDelete}}},
		{name: "versions of one user key", give: versionedEntries()},
		{name: "one hundred entries over the default block size", give: manyEntries(100)},
		{
			name:    "one hundred entries over tiny blocks",
			giveOpt: WriterOptions{BlockSize: 1, BloomBitsPerKey: 4},
			give:    manyEntries(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, meta := buildTable(t, tt.giveOpt, tt.give)

			if got, want := meta.Size, int64(len(data)); got != want {
				t.Errorf("Size = %d, want %d", got, want)
			}
			if got, want := meta.EntryCount, uint64(len(tt.give)); got != want {
				t.Errorf("EntryCount = %d, want %d", got, want)
			}
			if got, want := meta.Smallest, tt.give[0].ikey(); !bytes.Equal(got, want) {
				t.Errorf("Smallest = %x, want %x", got, want)
			}
			if got, want := meta.Largest, tt.give[len(tt.give)-1].ikey(); !bytes.Equal(got, want) {
				t.Errorf("Largest = %x, want %x", got, want)
			}
		})
	}
}

func TestWriter_CutsBlocksWhenTheBlockSizeIsReached(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		giveSize  int
		give      []tableEntry
		wantMulti int
	}{
		{name: "tiny blocks hold one entry each", giveSize: 1, give: manyEntries(8), wantMulti: 8},
		{name: "small blocks hold a few entries", giveSize: 128, give: manyEntries(20), wantMulti: 10},
		{name: "the default block size spans several blocks", give: manyEntries(100), wantMulti: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blockSize := tt.giveSize
			if blockSize == 0 {
				blockSize = _defaultBlockSize
			}

			data, _ := buildTable(t, WriterOptions{BlockSize: tt.giveSize}, tt.give)
			table := openTable(t, data)

			if got := len(table.index); got != tt.wantMulti {
				t.Fatalf("table holds %d blocks, want %d", got, tt.wantMulti)
			}

			offset := int64(0)
			for i, e := range table.index {
				if e.offset != offset {
					t.Fatalf("block %d starts at %d, want %d", i, e.offset, offset)
				}
				if payload := e.length - _blockTrailerSize; i < len(table.index)-1 && payload < blockSize {
					t.Errorf("block %d holds %d payload bytes, want at least the %d-byte block size", i, payload, blockSize)
				}
				offset += int64(e.length)
			}

			if got, want := uint64(offset), tableFooter(t, data).filterOffset; got != want {
				t.Errorf("data blocks end at %d, want the filter offset %d", got, want)
			}
		})
	}
}

func TestWriter_ProducesIdenticalBytesForIdenticalEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		giveOpt WriterOptions
		give    []tableEntry
	}{
		{name: "golden entries over tiny blocks", giveOpt: WriterOptions{BlockSize: 64}, give: goldenEntries()},
		{name: "one hundred entries over default options", give: manyEntries(100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			first, firstMeta := buildTable(t, tt.giveOpt, tt.give)
			second, secondMeta := buildTable(t, tt.giveOpt, tt.give)

			if !bytes.Equal(first, second) {
				t.Errorf("second write = %x, want %x", second, first)
			}
			if firstMeta.Size != secondMeta.Size || firstMeta.EntryCount != secondMeta.EntryCount {
				t.Errorf("second meta = %+v, want %+v", secondMeta, firstMeta)
			}
		})
	}
}

func TestWriter_ReturnsFileWriteErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		giveAfter int
	}{
		{name: "first data block", giveAfter: 0},
		{name: "second data block", giveAfter: 1},
		{name: "last data block flushed by Finish", giveAfter: 2},
		{name: "filter block", giveAfter: 3},
		{name: "index block", giveAfter: 4},
		{name: "footer", giveAfter: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeFile{writeErr: errWrite, writeErrAfter: tt.giveAfter}
			w := NewWriter(f, WriterOptions{BlockSize: 64})

			var err error
			for _, e := range goldenEntries() {
				if err = w.Add(e.ikey(), []byte(e.value)); err != nil {
					break
				}
			}
			if err == nil {
				_, err = w.Finish()
			}

			if !errors.Is(err, errWrite) {
				t.Fatalf("error = %v, want %v", err, errWrite)
			}
		})
	}
}

func TestWriter_PanicsOnMisuse(t *testing.T) {
	t.Parallel()

	entry := tableEntry{user: "key", seq: 2, kind: keys.KindSet, value: "v"}

	t.Run("finish on an empty table", func(t *testing.T) {
		t.Parallel()

		w := NewWriter(&fakeFile{}, WriterOptions{})
		wantPanic(t, "Finish on an empty writer", func() { _, _ = w.Finish() })
	})

	t.Run("add after finish", func(t *testing.T) {
		t.Parallel()

		w := NewWriter(&fakeFile{}, WriterOptions{})
		if err := w.Add(entry.ikey(), []byte(entry.value)); err != nil {
			t.Fatalf("Add: %v", err)
		}
		if _, err := w.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}

		wantPanic(t, "Add after Finish", func() { _ = w.Add(entry.ikey(), nil) })
		wantPanic(t, "Finish after Finish", func() { _, _ = w.Finish() })
	})

	t.Run("keys out of order", func(t *testing.T) {
		t.Parallel()

		w := NewWriter(&fakeFile{}, WriterOptions{})
		if err := w.Add(entry.ikey(), []byte(entry.value)); err != nil {
			t.Fatalf("Add: %v", err)
		}

		earlier := tableEntry{user: "abc", seq: 1, kind: keys.KindSet}
		wantPanic(t, "Add of an earlier key", func() { _ = w.Add(earlier.ikey(), nil) })
	})

	t.Run("duplicate key", func(t *testing.T) {
		t.Parallel()

		w := NewWriter(&fakeFile{}, WriterOptions{})
		if err := w.Add(entry.ikey(), []byte(entry.value)); err != nil {
			t.Fatalf("Add: %v", err)
		}

		wantPanic(t, "Add of a repeated key", func() { _ = w.Add(entry.ikey(), nil) })
	})
}
