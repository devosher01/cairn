package wal

import (
	"encoding/binary"
	"hash/crc32"
)

const MaxPayload = 64 << 20

const (
	_magic       = "CAIRNWAL"
	_version     = 1
	_magicSize   = 8
	_versionSize = 4
	_headerSize  = _magicSize + _versionSize

	_crcSize         = 4
	_lengthSize      = 4
	_frameHeaderSize = _crcSize + _lengthSize
)

var _crcTable = crc32.MakeTable(crc32.Castagnoli)

func encodeFileHeader(dst []byte) {
	copy(dst, _magic)
	binary.LittleEndian.PutUint32(dst[_magicSize:_headerSize], _version)
}

func validFileHeader(src []byte) bool {
	return string(src[:_magicSize]) == _magic &&
		binary.LittleEndian.Uint32(src[_magicSize:_headerSize]) == _version
}

func encodeFrame(dst, payload []byte) {
	binary.LittleEndian.PutUint32(dst[_crcSize:_frameHeaderSize], uint32(len(payload)))
	copy(dst[_frameHeaderSize:], payload)
	binary.LittleEndian.PutUint32(dst[:_crcSize], frameChecksum(dst[_crcSize:_frameHeaderSize], payload))
}

func frameChecksum(lengthField, payload []byte) uint32 {
	return crc32.Update(crc32.Update(0, _crcTable, lengthField), _crcTable, payload)
}

func frameCRC(frameHeader []byte) uint32 {
	return binary.LittleEndian.Uint32(frameHeader[:_crcSize])
}

func frameLength(frameHeader []byte) int64 {
	return int64(binary.LittleEndian.Uint32(frameHeader[_crcSize:_frameHeaderSize]))
}
