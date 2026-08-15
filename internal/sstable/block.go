package sstable

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	_blockTrailerSize = 5
	_blockTypeRaw     = 0
)

var errBlockCorrupt = errors.New("sstable: block corrupt")

var _blockCRC = crc32.MakeTable(crc32.Castagnoli)

type blockBuilder struct {
	payload []byte
	last    []byte
}

func newBlockBuilder() *blockBuilder {
	return &blockBuilder{}
}

func (b *blockBuilder) add(ikey, value []byte) {
	b.payload = binary.AppendUvarint(b.payload, uint64(len(ikey)))
	b.payload = binary.AppendUvarint(b.payload, uint64(len(value)))
	b.payload = append(b.payload, ikey...)
	b.payload = append(b.payload, value...)
	b.last = append(b.last[:0], ikey...)
}

func (b *blockBuilder) empty() bool {
	return len(b.payload) == 0
}

func (b *blockBuilder) size() int {
	return len(b.payload)
}

func (b *blockBuilder) lastKey() []byte {
	return b.last
}

func (b *blockBuilder) finish() []byte {
	return b.payload
}

func (b *blockBuilder) reset() {
	b.payload = b.payload[:0]
	b.last = b.last[:0]
}

func sealBlock(payload []byte) []byte {
	raw := make([]byte, 0, len(payload)+_blockTrailerSize)
	raw = append(raw, payload...)
	raw = append(raw, _blockTypeRaw)
	sum := crc32.Checksum(raw, _blockCRC)

	return binary.LittleEndian.AppendUint32(raw, sum)
}

func verifyBlock(raw []byte) ([]byte, error) {
	if len(raw) < _blockTrailerSize {
		return nil, fmt.Errorf("%w: stored block of %d bytes is shorter than its %d-byte trailer",
			errBlockCorrupt, len(raw), _blockTrailerSize)
	}

	payload := raw[:len(raw)-_blockTrailerSize]
	body := raw[:len(raw)-crc32.Size]
	if kind := body[len(body)-1]; kind != _blockTypeRaw {
		return nil, fmt.Errorf("%w: block type %d is unknown", errBlockCorrupt, kind)
	}

	stored := binary.LittleEndian.Uint32(raw[len(raw)-crc32.Size:])
	if sum := crc32.Checksum(body, _blockCRC); sum != stored {
		return nil, fmt.Errorf("%w: block crc %#08x does not match the stored %#08x", errBlockCorrupt, sum, stored)
	}

	return payload, nil
}
