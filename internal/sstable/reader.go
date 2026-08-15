package sstable

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"sync/atomic"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
)

type Table struct {
	file   env.File
	index  []indexEntry
	filter []byte
	allErr atomic.Pointer[error]
}

func Open(f env.File, size int64) (*Table, error) {
	if size < _footerSize {
		return nil, fmt.Errorf("%w: table of %d bytes is shorter than the %d-byte footer", ErrCorrupt, size, _footerSize)
	}

	var raw [_footerSize]byte
	if err := readFull(f, raw[:], size-_footerSize); err != nil {
		return nil, err
	}
	ft, err := decodeFooter(raw[:])
	if err != nil {
		return nil, err
	}

	limit := size - _footerSize
	index, err := readSealedBlock(f, ft.indexOffset, ft.indexLength, limit, "index block")
	if err != nil {
		return nil, err
	}
	entries, err := parseIndex(index, limit)
	if err != nil {
		return nil, err
	}
	filter, err := readSealedBlock(f, ft.filterOffset, ft.filterLength, limit, "filter block")
	if err != nil {
		return nil, err
	}

	return &Table{file: f, index: entries, filter: bytes.Clone(filter)}, nil
}

func (t *Table) Get(user []byte, seq keys.Seq) ([]byte, keys.Kind, bool, error) {
	if !filterContains(t.filter, filterHash(user)) {
		return nil, 0, false, nil
	}

	seek := keys.AppendSeek(nil, user, seq)
	at, _ := slices.BinarySearchFunc(t.index, seek, func(e indexEntry, target []byte) int {
		return keys.Compare(e.lastKey, target)
	})
	if at == len(t.index) {
		return nil, 0, false, nil
	}

	entry := t.index[at]
	payload, err := t.readBlock(entry, make([]byte, entry.length))
	if err != nil {
		return nil, 0, false, err
	}

	it := newBlockIter(payload)
	for {
		ikey, value, ok := it.next()
		if !ok {
			break
		}
		if !validIKey(ikey) {
			return nil, 0, false, shortKeyErr(entry.offset, len(ikey))
		}
		if keys.Compare(ikey, seek) < 0 {
			continue
		}
		if !bytes.Equal(keys.UserKey(ikey), user) {
			return nil, 0, false, nil
		}

		_, kind := keys.Trailer(ikey)
		if !kind.Valid() {
			return nil, 0, false, fmt.Errorf("%w: data block at offset %d holds kind %d", ErrCorrupt, entry.offset, kind)
		}

		return value, kind, true, nil
	}
	if err := it.err(); err != nil {
		return nil, 0, false, blockErr(entry.offset, err)
	}

	return nil, 0, false, nil
}

func (t *Table) All() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		t.allErr.Store(nil)

		var buf []byte
		for _, entry := range t.index {
			if cap(buf) < entry.length {
				buf = make([]byte, entry.length)
			}
			payload, err := t.readBlock(entry, buf[:entry.length])
			if err != nil {
				t.allErr.Store(&err)

				return
			}

			it := newBlockIter(payload)
			for {
				ikey, value, ok := it.next()
				if !ok {
					break
				}
				if !validIKey(ikey) {
					err := shortKeyErr(entry.offset, len(ikey))
					t.allErr.Store(&err)

					return
				}
				if !yield(ikey, value) {
					return
				}
			}
			if err := it.err(); err != nil {
				err = blockErr(entry.offset, err)
				t.allErr.Store(&err)

				return
			}
		}
	}
}

func (t *Table) AllErr() error {
	if err := t.allErr.Load(); err != nil {
		return *err
	}

	return nil
}

func (t *Table) Close() error {
	if err := t.file.Close(); err != nil {
		return fmt.Errorf("sstable: close: %w", err)
	}

	return nil
}

func (t *Table) readBlock(entry indexEntry, dst []byte) ([]byte, error) {
	if err := readFull(t.file, dst, entry.offset); err != nil {
		return nil, err
	}
	payload, err := verifyBlock(dst)
	if err != nil {
		return nil, blockErr(entry.offset, err)
	}

	return payload, nil
}

func readSealedBlock(f env.File, offset, length uint64, limit int64, what string) ([]byte, error) {
	if !validSpan(offset, length, limit) {
		return nil, fmt.Errorf("%w: %s spans %d bytes at offset %d outside the %d-byte data region",
			ErrCorrupt, what, length, offset, limit)
	}

	raw := make([]byte, length)
	if err := readFull(f, raw, int64(offset)); err != nil {
		return nil, err
	}
	payload, err := verifyBlock(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrCorrupt, what, err)
	}

	return payload, nil
}

func readFull(f env.File, dst []byte, offset int64) error {
	n, err := f.ReadAt(dst, offset)
	if n == len(dst) {
		return nil
	}
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: read %d of %d bytes at offset %d", ErrCorrupt, n, len(dst), offset)
	}

	return fmt.Errorf("sstable: read %d bytes at offset %d: %w", len(dst), offset, err)
}

func validIKey(ikey []byte) bool {
	return len(ikey) > keys.TrailerSize
}
