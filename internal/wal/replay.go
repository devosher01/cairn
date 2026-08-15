package wal

import (
	"errors"
	"fmt"
	"io"

	"github.com/devosher01/cairn/internal/env"
)

func Replay(f env.File, size int64, apply func(payload []byte) error) (int64, error) {
	if size < _headerSize {
		return 0, nil
	}

	var header [_headerSize]byte
	complete, err := readAtFull(f, header[:], 0)
	if err != nil {
		return 0, fmt.Errorf("wal: read header: %w", err)
	}
	if !complete || !validFileHeader(header[:]) {
		return 0, nil
	}

	var (
		frameHeader [_frameHeaderSize]byte
		buf         []byte
		offset      = int64(_headerSize)
	)
	for size-offset >= _frameHeaderSize {
		complete, err := readAtFull(f, frameHeader[:], offset)
		if err != nil {
			return offset, fmt.Errorf("wal: read record header at offset %d: %w", offset, err)
		}
		if !complete {
			return offset, nil
		}

		length := frameLength(frameHeader[:])
		if length > MaxPayload || offset+_frameHeaderSize+length > size {
			return offset, nil
		}

		if int64(cap(buf)) < length {
			buf = make([]byte, length)
		}
		payload := buf[:length]

		complete, err = readAtFull(f, payload, offset+_frameHeaderSize)
		if err != nil {
			return offset, fmt.Errorf("wal: read record payload at offset %d: %w", offset, err)
		}
		if !complete {
			return offset, nil
		}

		if frameChecksum(frameHeader[_crcSize:], payload) != frameCRC(frameHeader[:]) {
			return offset, nil
		}

		if err := apply(payload); err != nil {
			return offset, fmt.Errorf("wal: apply record at offset %d: %w", offset, err)
		}
		offset += _frameHeaderSize + length
	}

	return offset, nil
}

func readAtFull(f env.File, dst []byte, offset int64) (bool, error) {
	n, err := f.ReadAt(dst, offset)
	if n == len(dst) {
		return true, nil
	}
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return false, nil
	}

	return false, err
}
