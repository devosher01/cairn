package wal_test

import (
	"io"
	"slices"

	"github.com/devosher01/cairn/internal/env"
)

var _ env.File = (*fakeFile)(nil)

type fakeFile struct {
	data     []byte
	writes   [][]byte
	syncs    int
	closes   int
	writeErr error

	reads        int
	readErr      error
	readErrAfter int

	syncErr  error
	closeErr error
}

func (f *fakeFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.writes = append(f.writes, slices.Clone(p))
	f.data = append(f.data, p...)

	return len(p), nil
}

func (f *fakeFile) ReadAt(p []byte, off int64) (int, error) {
	f.reads++
	if f.readErr != nil {
		if f.readErrAfter == 0 {
			return 0, f.readErr
		}
		f.readErrAfter--
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
	f.syncs++

	return f.syncErr
}

func (f *fakeFile) Close() error {
	f.closes++

	return f.closeErr
}

func (f *fakeFile) Size() (int64, error) {
	return int64(len(f.data)), nil
}
