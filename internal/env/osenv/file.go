package osenv

import (
	"os"

	"github.com/devosher01/cairn/internal/env"
)

type file struct {
	f *os.File
}

var _ env.File = (*file)(nil)

func (f *file) ReadAt(p []byte, off int64) (int, error) {
	return f.f.ReadAt(p, off)
}

func (f *file) Write(p []byte) (int, error) {
	return f.f.Write(p)
}

func (f *file) Sync() error {
	return f.f.Sync()
}

func (f *file) Close() error {
	return f.f.Close()
}

func (f *file) Size() (int64, error) {
	st, err := f.f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
