package sstable

import (
	"encoding/binary"
	"fmt"
	"math"
)

type blockIter struct {
	payload []byte
	pos     int
	fail    error
}

func newBlockIter(payload []byte) *blockIter {
	return &blockIter{payload: payload}
}

func (i *blockIter) next() ([]byte, []byte, bool) {
	if i.fail != nil || i.pos >= len(i.payload) {
		return nil, nil, false
	}

	klen, ok := i.length("key length")
	if !ok {
		return nil, nil, false
	}
	vlen, ok := i.length("value length")
	if !ok {
		return nil, nil, false
	}
	ikey, ok := i.slice(klen, "key")
	if !ok {
		return nil, nil, false
	}
	value, ok := i.slice(vlen, "value")
	if !ok {
		return nil, nil, false
	}

	return ikey, value, true
}

func (i *blockIter) err() error {
	return i.fail
}

func (i *blockIter) length(field string) (uint64, bool) {
	n, read := binary.Uvarint(i.payload[i.pos:])
	if read <= 0 {
		i.fail = fmt.Errorf("%w: %s at offset %d is a truncated or overlong uvarint", errBlockCorrupt, field, i.pos)
		return 0, false
	}
	if n > math.MaxUint32 {
		i.fail = fmt.Errorf("%w: %s %d at offset %d exceeds the 32-bit length limit", errBlockCorrupt, field, n, i.pos)
		return 0, false
	}
	i.pos += read

	return n, true
}

func (i *blockIter) slice(n uint64, field string) ([]byte, bool) {
	if n > uint64(len(i.payload)-i.pos) {
		i.fail = fmt.Errorf("%w: %s of %d bytes at offset %d overruns the %d-byte payload",
			errBlockCorrupt, field, n, i.pos, len(i.payload))
		return nil, false
	}

	b := i.payload[i.pos : i.pos+int(n)]
	i.pos += int(n)

	return b, true
}
