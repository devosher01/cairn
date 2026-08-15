package wal

import (
	"fmt"

	"github.com/devosher01/cairn/internal/env"
)

type Writer struct {
	file env.File
	buf  []byte
	size int64
}

func NewWriter(f env.File) (*Writer, error) {
	var header [_headerSize]byte
	encodeFileHeader(header[:])

	if _, err := f.Write(header[:]); err != nil {
		return nil, fmt.Errorf("wal: write header: %w", err)
	}

	return &Writer{file: f, size: _headerSize}, nil
}

func (w *Writer) Append(payload []byte) error {
	size := _frameHeaderSize + len(payload)
	if cap(w.buf) < size {
		w.buf = make([]byte, size)
	}
	w.buf = w.buf[:size]
	encodeFrame(w.buf, payload)

	if _, err := w.file.Write(w.buf); err != nil {
		return fmt.Errorf("wal: append record: %w", err)
	}
	w.size += int64(size)

	return nil
}

func (w *Writer) Sync() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: sync: %w", err)
	}

	return nil
}

func (w *Writer) Size() int64 {
	return w.size
}

func (w *Writer) Close() error {
	err := w.Sync()
	if closeErr := w.file.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("wal: close: %w", closeErr)
	}

	return err
}
