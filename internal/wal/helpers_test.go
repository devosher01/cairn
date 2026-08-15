package wal_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/wal"
)

var (
	errBoom  = errors.New("boom")
	errOther = errors.New("other")
)

func buildWAL(t *testing.T, payloads ...[]byte) []byte {
	t.Helper()

	f := &fakeFile{}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}
	for _, payload := range payloads {
		if err := w.Append(payload); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	return f.data
}

func replayAll(data []byte) ([][]byte, int64, error) {
	var got [][]byte
	f := &fakeFile{data: data}
	offset, err := wal.Replay(f, int64(len(data)), func(payload []byte) error {
		got = append(got, slices.Clone(payload))

		return nil
	})

	return got, offset, err
}

func equalPayloads(got, want [][]byte) bool {
	return slices.EqualFunc(got, want, bytes.Equal)
}

func flipByte(data []byte, offset int) []byte {
	out := slices.Clone(data)
	out[offset] ^= 0xff

	return out
}
