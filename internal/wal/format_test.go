package wal_test

import (
	"bytes"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata")

func TestFrame_MatchesFixedCRCVector(t *testing.T) {
	t.Parallel()

	want, err := hex.DecodeString("f883145503000000616263")
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	got := buildWAL(t, []byte("abc"))[12:]
	if !bytes.Equal(got, want) {
		t.Errorf("got frame %x, want %x", got, want)
	}
}

func TestFileHeader_MatchesFixedVector(t *testing.T) {
	t.Parallel()

	want, err := hex.DecodeString("434149524e57414c01000000")
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	got := buildWAL(t)
	if !bytes.Equal(got, want) {
		t.Errorf("got header %x, want %x", got, want)
	}
}

func TestWAL_MatchesGoldenFile(t *testing.T) {
	t.Parallel()

	got := buildWAL(t,
		nil,
		[]byte("a"),
		[]byte("cairn wal golden vector"),
		bytes.Repeat([]byte{0xab}, 40),
	)

	path := filepath.Join("testdata", "wal_v1.golden")
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}
