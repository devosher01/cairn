package batch

import (
	"encoding/binary"
	"slices"

	"github.com/devosher01/cairn/internal/keys"
)

type Batch struct {
	buf []byte
}

func New() *Batch {
	return &Batch{buf: make([]byte, _headerSize)}
}

func (b *Batch) Put(key, value []byte) {
	b.appendKey(keys.KindSet, key)
	b.buf = binary.AppendUvarint(b.buf, uint64(len(value)))
	b.buf = append(b.buf, value...)
	b.incCount()
}

func (b *Batch) Delete(key []byte) {
	b.appendKey(keys.KindDelete, key)
	b.incCount()
}

func (b *Batch) Count() uint32 {
	return count(b.buf)
}

func (b *Batch) Len() int {
	return len(b.buf)
}

func (b *Batch) Reset() {
	b.buf = slices.Grow(b.buf[:0], _headerSize)[:_headerSize]
	clear(b.buf)
}

func (b *Batch) Seal(seqBase keys.Seq) []byte {
	putSeqBase(b.buf, seqBase)

	return b.buf
}

func (b *Batch) appendKey(kind keys.Kind, key []byte) {
	b.buf = append(b.buf, byte(kind))
	b.buf = binary.AppendUvarint(b.buf, uint64(len(key)))
	b.buf = append(b.buf, key...)
}

func (b *Batch) incCount() {
	putCount(b.buf, b.Count()+1)
}
