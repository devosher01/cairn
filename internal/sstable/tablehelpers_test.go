package sstable

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
)

var (
	errWrite = errors.New("write failed")
	errRead  = errors.New("read failed")
	errClose = errors.New("close failed")
)

var _ env.File = (*fakeFile)(nil)

type fakeFile struct {
	data []byte

	writeErr      error
	writeErrAfter int

	readErr      error
	readErrAfter atomic.Int64

	closes   int
	closeErr error
}

func failingReadFile(data []byte, after int64) *fakeFile {
	f := &fakeFile{data: data, readErr: errRead}
	f.readErrAfter.Store(after)

	return f
}

func (f *fakeFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		if f.writeErrAfter == 0 {
			return 0, f.writeErr
		}
		f.writeErrAfter--
	}
	f.data = append(f.data, p...)

	return len(p), nil
}

func (f *fakeFile) ReadAt(p []byte, off int64) (int, error) {
	if f.readErr != nil && f.readErrAfter.Add(-1) < 0 {
		return 0, f.readErr
	}
	if off < 0 || off > int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

func (f *fakeFile) Sync() error {
	return nil
}

func (f *fakeFile) Close() error {
	f.closes++

	return f.closeErr
}

func (f *fakeFile) Size() (int64, error) {
	return int64(len(f.data)), nil
}

type tableEntry struct {
	user  string
	seq   keys.Seq
	kind  keys.Kind
	value string
}

func (e tableEntry) ikey() []byte {
	return keys.Append(nil, []byte(e.user), e.seq, e.kind)
}

func buildTable(t *testing.T, opts WriterOptions, entries []tableEntry) ([]byte, Meta) {
	t.Helper()

	f := &fakeFile{}
	w := NewWriter(f, opts)
	for _, e := range entries {
		if err := w.Add(e.ikey(), []byte(e.value)); err != nil {
			t.Fatalf("Add(%q, %d): %v", e.user, e.seq, err)
		}
	}
	meta, err := w.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	return f.data, meta
}

func openTable(t *testing.T, data []byte) *Table {
	t.Helper()

	table, err := Open(&fakeFile{data: data}, int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := table.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	return table
}

func dumpTable(t *testing.T, table *Table) []tableEntry {
	t.Helper()

	var got []tableEntry
	for ikey, value := range table.All() {
		seq, kind := keys.Trailer(ikey)
		got = append(got, tableEntry{
			user:  string(keys.UserKey(ikey)),
			seq:   seq,
			kind:  kind,
			value: string(value),
		})
	}
	if err := table.AllErr(); err != nil {
		t.Fatalf("AllErr: %v", err)
	}

	return got
}

func tableFooter(t *testing.T, data []byte) footer {
	t.Helper()

	ft, err := decodeFooter(data[len(data)-_footerSize:])
	if err != nil {
		t.Fatalf("decodeFooter: %v", err)
	}

	return ft
}

func rewriteFooter(data []byte, ft footer) []byte {
	out := slices.Clone(data)
	encodeFooter(out[len(out)-_footerSize:], ft)

	return out
}

func setFooterVersion(data []byte, version uint32) []byte {
	out := slices.Clone(data)
	ft := out[len(out)-_footerSize:]
	binary.LittleEndian.PutUint32(ft[_footerVersionAt:_footerCRCAt], version)
	binary.LittleEndian.PutUint32(ft[_footerCRCAt:_footerMagicAt], crc32.Checksum(ft[:_footerCRCAt], _blockCRC))

	return out
}

type craftedIndexEntry struct {
	lastKey []byte
	handle  []byte
}

func withCraftedIndex(t *testing.T, data []byte, entries []craftedIndexEntry) []byte {
	t.Helper()

	b := newBlockBuilder()
	for _, e := range entries {
		b.add(e.lastKey, e.handle)
	}

	return withCraftedIndexPayload(t, data, b.finish())
}

func withCraftedBlock(t *testing.T, data, payload, lastKey []byte) []byte {
	t.Helper()

	ft := tableFooter(t, data)
	block := sealBlock(payload)

	out := slices.Clone(data[:len(data)-_footerSize])
	offset := uint64(len(out))
	out = append(out, block...)

	b := newBlockBuilder()
	b.add(lastKey, appendHandle(nil, offset, uint64(len(block))))
	index := sealBlock(b.finish())
	indexOffset := uint64(len(out))
	out = append(out, index...)

	var raw [_footerSize]byte
	encodeFooter(raw[:], footer{
		indexOffset:  indexOffset,
		indexLength:  uint64(len(index)),
		filterOffset: ft.filterOffset,
		filterLength: ft.filterLength,
	})

	return append(out, raw[:]...)
}

func withCraftedIndexPayload(t *testing.T, data, payload []byte) []byte {
	t.Helper()

	ft := tableFooter(t, data)
	index := sealBlock(payload)

	out := slices.Clone(data[:len(data)-_footerSize])
	indexOffset := uint64(len(out))
	out = append(out, index...)

	var raw [_footerSize]byte
	encodeFooter(raw[:], footer{
		indexOffset:  indexOffset,
		indexLength:  uint64(len(index)),
		filterOffset: ft.filterOffset,
		filterLength: ft.filterLength,
	})

	return append(out, raw[:]...)
}

func bloomPositiveKey(t *testing.T, table *Table, prefix string) string {
	t.Helper()

	for i := range 1000 {
		candidate := fmt.Sprintf("%s%04d", prefix, i)
		if filterContains(table.filter, filterHash([]byte(candidate))) {
			return candidate
		}
	}
	t.Fatalf("no absent user key with prefix %q passes the bloom filter", prefix)

	return ""
}

func manyEntries(n int) []tableEntry {
	out := make([]tableEntry, 0, n)
	for i := range n {
		out = append(out, tableEntry{
			user:  fmt.Sprintf("key%04d", i),
			seq:   keys.Seq(i + 1),
			kind:  keys.KindSet,
			value: fmt.Sprintf("value%04d%s", i, strings.Repeat("-", 100)),
		})
	}

	return out
}

func versionedEntries() []tableEntry {
	return []tableEntry{
		{user: "apple", seq: 4, kind: keys.KindSet, value: "apple v4"},
		{user: "pear", seq: 30, kind: keys.KindSet, value: "pear v30"},
		{user: "pear", seq: 20, kind: keys.KindDelete},
		{user: "pear", seq: 10, kind: keys.KindSet, value: "pear v10"},
		{user: "plum", seq: 7, kind: keys.KindSet, value: "plum v7"},
	}
}

func goldenEntries() []tableEntry {
	return []tableEntry{
		{user: "alpha", seq: 9, kind: keys.KindSet, value: "value for alpha at 9"},
		{user: "alpha", seq: 4, kind: keys.KindDelete},
		{user: "beta", seq: 7, kind: keys.KindSet, value: "value for beta at 7"},
		{user: "delta", seq: 12, kind: keys.KindSet},
		{user: "epsilon", seq: 3, kind: keys.KindSet, value: "value for epsilon at 3"},
		{user: "gamma", seq: 5, kind: keys.KindDelete},
		{user: "omega", seq: 1, kind: keys.KindSet, value: "value for omega at 1"},
	}
}

func goldenPath() string {
	return filepath.Join("testdata", "sst_v1.golden")
}

func goldenTable(t testing.TB) []byte {
	t.Helper()

	data, err := os.ReadFile(goldenPath())
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	return data
}

func wantPanic(t *testing.T, what string, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()

	fn()
}
