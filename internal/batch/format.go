package batch

import (
	"encoding/binary"

	"github.com/devosher01/cairn/internal/keys"
)

const (
	_seqBaseSize = 8
	_countSize   = 4
	_headerSize  = _seqBaseSize + _countSize
)

func putSeqBase(dst []byte, seq keys.Seq) {
	binary.LittleEndian.PutUint64(dst[:_seqBaseSize], uint64(seq))
}

func seqBase(src []byte) keys.Seq {
	return keys.Seq(binary.LittleEndian.Uint64(src[:_seqBaseSize]))
}

func putCount(dst []byte, n uint32) {
	binary.LittleEndian.PutUint32(dst[_seqBaseSize:_headerSize], n)
}

func count(src []byte) uint32 {
	return binary.LittleEndian.Uint32(src[_seqBaseSize:_headerSize])
}
