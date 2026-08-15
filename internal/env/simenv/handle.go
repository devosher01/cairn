package simenv

import (
	"io"

	"github.com/devosher01/cairn/internal/env"
)

type writeHandle struct {
	fs   *FS
	file *file
	off  int64
}

var _ env.File = (*writeHandle)(nil)

func (h *writeHandle) Write(p []byte) (int, error) {
	return h.fs.write(h, p)
}

func (h *writeHandle) Sync() error {
	return h.fs.syncFile(h.file)
}

func (h *writeHandle) Size() (int64, error) {
	h.fs.mu.Lock()
	defer h.fs.mu.Unlock()

	return int64(len(h.file.data)), nil
}

func (h *writeHandle) Close() error {
	return nil
}

func (h *writeHandle) ReadAt([]byte, int64) (int, error) {
	panic("simenv: ReadAt on a write handle")
}

type readHandle struct {
	fs   *FS
	file *file
}

var _ env.File = (*readHandle)(nil)

func (h *readHandle) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		panic("simenv: ReadAt with a negative offset")
	}

	h.fs.mu.Lock()
	defer h.fs.mu.Unlock()

	data := h.file.data
	if off >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *readHandle) Size() (int64, error) {
	h.fs.mu.Lock()
	defer h.fs.mu.Unlock()

	return int64(len(h.file.data)), nil
}

func (h *readHandle) Close() error {
	return nil
}

func (h *readHandle) Write([]byte) (int, error) {
	panic("simenv: Write on a read handle")
}

func (h *readHandle) Sync() error {
	panic("simenv: Sync on a read handle")
}
