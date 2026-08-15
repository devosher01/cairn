package manifest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"

	"github.com/devosher01/cairn/internal/keys"
)

func Decode(raw []byte) (State, error) {
	if len(raw) < _minSize {
		return State{}, fmt.Errorf("manifest: file of %d bytes: %w", len(raw), ErrCorrupt)
	}
	if string(raw[:_magicSize]) != _magic {
		return State{}, fmt.Errorf("manifest: magic %q: %w", raw[:_magicSize], ErrCorrupt)
	}
	if v := binary.LittleEndian.Uint32(raw[_magicSize:_headerSize]); v != _version {
		return State{}, fmt.Errorf("manifest: version %d: %w", v, ErrCorrupt)
	}

	body := raw[:len(raw)-_crcSize]
	want := binary.LittleEndian.Uint32(raw[len(raw)-_crcSize:])
	if got := crc32.Checksum(body, _crcTable); got != want {
		return State{}, fmt.Errorf("manifest: checksum %08x, want %08x: %w", got, want, ErrCorrupt)
	}

	d := decoder{body: body, pos: _headerSize}

	return d.state()
}

type decoder struct {
	body []byte
	pos  int
}

func (d *decoder) state() (State, error) {
	var (
		s   State
		err error
	)
	if s.NextFileNum, err = d.uint64("next file number"); err != nil {
		return State{}, err
	}

	seq, err := d.uint64("last sequence")
	if err != nil {
		return State{}, err
	}
	s.LastSeq = keys.Seq(seq)

	if s.OldestWAL, err = d.uint64("oldest wal"); err != nil {
		return State{}, err
	}

	levels, err := d.uint8("level count")
	if err != nil {
		return State{}, err
	}
	if levels != NumLevels {
		return State{}, d.fail(fmt.Sprintf("level count %d", levels))
	}

	for i := range s.Levels {
		if s.Levels[i], err = d.level(); err != nil {
			return State{}, err
		}
	}
	if d.pos != len(d.body) {
		return State{}, d.fail("trailing bytes")
	}

	return s, nil
}

func (d *decoder) level() ([]Table, error) {
	count, err := d.uint32("table count")
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	if uint64(count) > uint64((len(d.body)-d.pos)/_minTableSize) {
		return nil, d.fail(fmt.Sprintf("table count %d", count))
	}

	level := make([]Table, 0, count)
	for range count {
		t, err := d.table()
		if err != nil {
			return nil, err
		}
		level = append(level, t)
	}

	return level, nil
}

func (d *decoder) table() (Table, error) {
	fileNum, err := d.uint64("table file number")
	if err != nil {
		return Table{}, err
	}
	size, err := d.uint64("table size")
	if err != nil {
		return Table{}, err
	}
	entries, err := d.uint64("table entry count")
	if err != nil {
		return Table{}, err
	}
	smallest, err := d.key("smallest key")
	if err != nil {
		return Table{}, err
	}
	largest, err := d.key("largest key")
	if err != nil {
		return Table{}, err
	}
	if keys.Compare(smallest, largest) > 0 {
		return Table{}, d.fail("smallest key above largest key")
	}

	return Table{
		FileNum:    fileNum,
		Size:       size,
		EntryCount: entries,
		Smallest:   smallest,
		Largest:    largest,
	}, nil
}

func (d *decoder) key(what string) ([]byte, error) {
	raw, err := d.bytes(what)
	if err != nil {
		return nil, err
	}
	if len(raw) < _minKeySize {
		return nil, d.fail(fmt.Sprintf("%s of %d bytes", what, len(raw)))
	}

	return bytes.Clone(raw), nil
}

func (d *decoder) bytes(what string) ([]byte, error) {
	n, size := binary.Uvarint(d.body[d.pos:])
	if size <= 0 {
		return nil, d.fail(what + " length prefix")
	}
	if n > math.MaxUint32 {
		return nil, d.fail(fmt.Sprintf("%s length %d", what, n))
	}
	d.pos += size

	if n > uint64(len(d.body)-d.pos) {
		return nil, d.fail(fmt.Sprintf("%s length %d overruns the file", what, n))
	}
	out := d.body[d.pos : d.pos+int(n)]
	d.pos += int(n)

	return out, nil
}

func (d *decoder) uint64(what string) (uint64, error) {
	if len(d.body)-d.pos < _u64Size {
		return 0, d.truncated(what)
	}
	v := binary.LittleEndian.Uint64(d.body[d.pos:])
	d.pos += _u64Size

	return v, nil
}

func (d *decoder) uint32(what string) (uint32, error) {
	if len(d.body)-d.pos < _u32Size {
		return 0, d.truncated(what)
	}
	v := binary.LittleEndian.Uint32(d.body[d.pos:])
	d.pos += _u32Size

	return v, nil
}

func (d *decoder) uint8(what string) (uint8, error) {
	if len(d.body)-d.pos < 1 {
		return 0, d.truncated(what)
	}
	v := d.body[d.pos]
	d.pos++

	return v, nil
}

func (d *decoder) fail(what string) error {
	return fmt.Errorf("manifest: %s at offset %d: %w", what, d.pos, ErrCorrupt)
}

func (d *decoder) truncated(what string) error {
	return fmt.Errorf("manifest: %s truncated at offset %d: %w", what, d.pos, ErrCorrupt)
}
