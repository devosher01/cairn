package sstable

import (
	"bytes"
	"flag"
	"os"
	"slices"
	"testing"
)

var _updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata")

func TestTable_MatchesGoldenFile(t *testing.T) {
	t.Parallel()

	got, meta := buildTable(t, WriterOptions{BlockSize: 64, BloomBitsPerKey: 10}, goldenEntries())

	if *_updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath(), got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want := goldenTable(t)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
	if meta.Size != int64(len(want)) {
		t.Errorf("Size = %d, want %d", meta.Size, len(want))
	}

	table := openTable(t, want)
	if len(table.index) < 3 {
		t.Errorf("golden table holds %d blocks, want at least 3", len(table.index))
	}
	if entries := dumpTable(t, table); !slices.Equal(entries, goldenEntries()) {
		t.Errorf("golden table holds %+v, want %+v", entries, goldenEntries())
	}
}
