package batch

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/devosher01/cairn/internal/keys"
)

type Reader struct {
	payload []byte
	base    keys.Seq
	total   uint32
	index   uint32
	pos     int
	err     error
}

func NewReader(payload []byte) (*Reader, error) {
	if len(payload) < _headerSize {
		return nil, fmt.Errorf("batch: header truncated at %d bytes: %w", len(payload), ErrCorrupt)
	}

	return &Reader{
		payload: payload,
		base:    seqBase(payload),
		total:   count(payload),
		pos:     _headerSize,
	}, nil
}

func (r *Reader) SeqBase() keys.Seq {
	return r.base
}

func (r *Reader) Count() uint32 {
	return r.total
}

func (r *Reader) Next() (Entry, bool) {
	if r.err != nil {
		return Entry{}, false
	}
	if r.index == r.total {
		if r.pos != len(r.payload) {
			r.fail("trailing bytes")
		}

		return Entry{}, false
	}
	if r.pos == len(r.payload) {
		r.fail("missing entry")

		return Entry{}, false
	}

	kind := keys.Kind(r.payload[r.pos])
	if !kind.Valid() {
		r.fail("entry kind")

		return Entry{}, false
	}
	r.pos++

	key, ok := r.readBytes()
	if !ok {
		return Entry{}, false
	}

	var value []byte
	if kind == keys.KindSet {
		if value, ok = r.readBytes(); !ok {
			return Entry{}, false
		}
	}

	entry := Entry{
		Seq:   r.base + keys.Seq(r.index),
		Kind:  kind,
		Key:   key,
		Value: value,
	}
	r.index++

	return entry, true
}

func (r *Reader) Err() error {
	return r.err
}

func (r *Reader) readBytes() ([]byte, bool) {
	n, size := binary.Uvarint(r.payload[r.pos:])
	if size <= 0 {
		r.fail("length prefix")

		return nil, false
	}
	if n > math.MaxUint32 {
		r.fail("length out of range")

		return nil, false
	}
	r.pos += size

	if n > uint64(len(r.payload)-r.pos) {
		r.fail("length overruns payload")

		return nil, false
	}
	out := r.payload[r.pos : r.pos+int(n)]
	r.pos += int(n)

	return out, true
}

func (r *Reader) fail(what string) {
	r.err = fmt.Errorf("batch: %s at offset %d: %w", what, r.pos, ErrCorrupt)
}
