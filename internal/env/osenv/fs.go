package osenv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/devosher01/cairn/internal/env"
)

const _lockName = "LOCK"

type fileSystem struct {
	dir string
}

var _ env.FS = (*fileSystem)(nil)

func (f *fileSystem) Create(name string) (env.File, error) {
	h, err := os.OpenFile(f.path(name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, _filePerm)
	if err != nil {
		return nil, err
	}
	return &file{f: h}, nil
}

func (f *fileSystem) Open(name string) (env.File, error) {
	h, err := os.Open(f.path(name))
	if err != nil {
		return nil, err
	}
	return &file{f: h}, nil
}

func (f *fileSystem) Remove(name string) error {
	return os.Remove(f.path(name))
}

func (f *fileSystem) Rename(oldname, newname string) error {
	return os.Rename(f.path(oldname), f.path(newname))
}

func (f *fileSystem) List() ([]string, error) {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

func (f *fileSystem) SyncDir() error {
	d, err := os.Open(f.dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

func (f *fileSystem) Lock() (io.Closer, error) {
	path := f.path(_lockName)
	h, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, _filePerm)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(h.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = h.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("osenv: flock %s: %w", path, env.ErrLocked)
		}
		return nil, fmt.Errorf("osenv: flock %s: %w", path, err)
	}
	return &fileLock{f: h}, nil
}

func (f *fileSystem) path(name string) string {
	return filepath.Join(f.dir, name)
}
