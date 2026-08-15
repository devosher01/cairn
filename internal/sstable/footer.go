package sstable

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	_footerMagic   = "CAIRNSST"
	_footerVersion = 1

	_footerSize      = 48
	_footerVersionAt = 32
	_footerCRCAt     = 36
	_footerMagicAt   = 40
)

type footer struct {
	indexOffset  uint64
	indexLength  uint64
	filterOffset uint64
	filterLength uint64
}

func encodeFooter(dst []byte, f footer) {
	binary.LittleEndian.PutUint64(dst[0:8], f.indexOffset)
	binary.LittleEndian.PutUint64(dst[8:16], f.indexLength)
	binary.LittleEndian.PutUint64(dst[16:24], f.filterOffset)
	binary.LittleEndian.PutUint64(dst[24:_footerVersionAt], f.filterLength)
	binary.LittleEndian.PutUint32(dst[_footerVersionAt:_footerCRCAt], _footerVersion)
	binary.LittleEndian.PutUint32(dst[_footerCRCAt:_footerMagicAt], crc32.Checksum(dst[:_footerCRCAt], _blockCRC))
	copy(dst[_footerMagicAt:_footerSize], _footerMagic)
}

func decodeFooter(src []byte) (footer, error) {
	if magic := src[_footerMagicAt:_footerSize]; string(magic) != _footerMagic {
		return footer{}, fmt.Errorf("%w: footer magic %q is not %q", ErrCorrupt, magic, _footerMagic)
	}
	if v := binary.LittleEndian.Uint32(src[_footerVersionAt:_footerCRCAt]); v != _footerVersion {
		return footer{}, fmt.Errorf("%w: footer format version %d is not %d", ErrCorrupt, v, _footerVersion)
	}

	stored := binary.LittleEndian.Uint32(src[_footerCRCAt:_footerMagicAt])
	if sum := crc32.Checksum(src[:_footerCRCAt], _blockCRC); sum != stored {
		return footer{}, fmt.Errorf("%w: footer crc %#08x does not match the stored %#08x", ErrCorrupt, sum, stored)
	}

	return footer{
		indexOffset:  binary.LittleEndian.Uint64(src[0:8]),
		indexLength:  binary.LittleEndian.Uint64(src[8:16]),
		filterOffset: binary.LittleEndian.Uint64(src[16:24]),
		filterLength: binary.LittleEndian.Uint64(src[24:_footerVersionAt]),
	}, nil
}
