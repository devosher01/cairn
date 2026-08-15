package manifest_test

import (
	"io"

	"github.com/devosher01/cairn/internal/env"
)

var (
	_ env.FS   = (*fakeFS)(nil)
	_ env.File = (*fakeFile)(nil)
)

type fakeFS struct {
	file    *fakeFile
	openErr error
}

func (f *fakeFS) Create(string) (env.File, error) {
	return f.file, nil
}

func (f *fakeFS) Open(string) (env.File, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}

	return f.file, nil
}

func (f *fakeFS) Remove(string) error {
	return nil
}

func (f *fakeFS) Rename(string, string) error {
	return nil
}

func (f *fakeFS) List() ([]string, error) {
	return nil, nil
}

func (f *fakeFS) SyncDir() error {
	return nil
}

func (f *fakeFS) Lock() (io.Closer, error) {
	return nil, nil
}

type fakeFile struct {
	data       []byte
	size       int64
	shortWrite int
	sizeErr    error
	readErr    error
	closeErr   error
}

func (f *fakeFile) Write(p []byte) (int, error) {
	if f.shortWrite > 0 {
		return f.shortWrite, nil
	}
	f.data = append(f.data, p...)

	return len(p), nil
}

func (f *fakeFile) ReadAt(p []byte, off int64) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	if off < 0 || off >= int64(len(f.data)) {
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
	return f.closeErr
}

func (f *fakeFile) Size() (int64, error) {
	if f.sizeErr != nil {
		return 0, f.sizeErr
	}
	if f.size > 0 {
		return f.size, nil
	}

	return int64(len(f.data)), nil
}
