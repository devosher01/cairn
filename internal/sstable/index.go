package sstable

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/devosher01/cairn/internal/keys"
)

type indexEntry struct {
	lastKey []byte
	offset  int64
	length  int
}

func appendHandle(dst []byte, offset, length uint64) []byte {
	dst = binary.AppendUvarint(dst, offset)

	return binary.AppendUvarint(dst, length)
}

func parseIndex(payload []byte, limit int64) ([]indexEntry, error) {
	var entries []indexEntry

	it := newBlockIter(payload)
	for {
		ikey, handle, ok := it.next()
		if !ok {
			break
		}
		if !validIKey(ikey) {
			return nil, fmt.Errorf("%w: index entry %d holds a %d-byte key", ErrCorrupt, len(entries), len(ikey))
		}
		if len(entries) > 0 && keys.Compare(ikey, entries[len(entries)-1].lastKey) <= 0 {
			return nil, fmt.Errorf("%w: index entry %d key %x does not follow %x",
				ErrCorrupt, len(entries), ikey, entries[len(entries)-1].lastKey)
		}

		offset, length, err := decodeHandle(handle, len(entries))
		if err != nil {
			return nil, err
		}
		if !validSpan(offset, length, limit) {
			return nil, fmt.Errorf("%w: index entry %d spans %d bytes at offset %d outside the %d-byte data region",
				ErrCorrupt, len(entries), length, offset, limit)
		}

		entries = append(entries, indexEntry{
			lastKey: bytes.Clone(ikey),
			offset:  int64(offset),
			length:  int(length),
		})
	}
	if err := it.err(); err != nil {
		return nil, fmt.Errorf("%w: index block: %w", ErrCorrupt, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: index block holds no entries", ErrCorrupt)
	}

	return entries, nil
}

func decodeHandle(handle []byte, entry int) (uint64, uint64, error) {
	offset, read := binary.Uvarint(handle)
	if read <= 0 {
		return 0, 0, fmt.Errorf("%w: index entry %d handle offset is a truncated or overlong uvarint", ErrCorrupt, entry)
	}
	length, more := binary.Uvarint(handle[read:])
	if more <= 0 {
		return 0, 0, fmt.Errorf("%w: index entry %d handle length is a truncated or overlong uvarint", ErrCorrupt, entry)
	}
	if rest := len(handle) - read - more; rest != 0 {
		return 0, 0, fmt.Errorf("%w: index entry %d handle carries %d trailing bytes", ErrCorrupt, entry, rest)
	}

	return offset, length, nil
}

func validSpan(offset, length uint64, limit int64) bool {
	return length >= _blockTrailerSize && length <= uint64(limit) && offset <= uint64(limit)-length
}
