package sstable

import (
	"slices"

	"github.com/devosher01/cairn/internal/keys"
)

type TableIter struct {
	table *Table
	buf   []byte
	block *blockIter
	at    int
	ikey  []byte
	value []byte
	fail  error
}

func (t *Table) Iter() *TableIter {
	return &TableIter{table: t}
}

func (i *TableIter) SeekGE(ikey []byte) {
	i.restart()

	at, _ := slices.BinarySearchFunc(i.table.index, ikey, func(e indexEntry, target []byte) int {
		return keys.Compare(e.lastKey, target)
	})
	if at == len(i.table.index) || !i.load(at) {
		return
	}

	for i.advance() {
		if keys.Compare(i.ikey, ikey) >= 0 {
			return
		}
	}
}

func (i *TableIter) First() {
	i.restart()

	if !i.load(0) {
		return
	}
	i.advance()
}

func (i *TableIter) Next() {
	if !i.Valid() {
		return
	}
	i.advance()
}

func (i *TableIter) Valid() bool {
	return i.ikey != nil
}

func (i *TableIter) Key() []byte {
	if !i.Valid() {
		panic("sstable: Key on an invalid iterator")
	}

	return i.ikey
}

func (i *TableIter) Value() []byte {
	if !i.Valid() {
		panic("sstable: Value on an invalid iterator")
	}

	return i.value
}

func (i *TableIter) Err() error {
	return i.fail
}

func (i *TableIter) advance() bool {
	for {
		ikey, value, ok := i.block.next()
		if ok {
			return i.accept(ikey, value)
		}
		if err := i.block.err(); err != nil {
			i.stop(blockErr(i.table.index[i.at].offset, err))

			return false
		}
		if i.at+1 == len(i.table.index) {
			i.invalidate()

			return false
		}
		if !i.load(i.at + 1) {
			return false
		}
	}
}

func (i *TableIter) accept(ikey, value []byte) bool {
	offset := i.table.index[i.at].offset
	if !validIKey(ikey) {
		i.stop(shortKeyErr(offset, len(ikey)))

		return false
	}
	if _, kind := keys.Trailer(ikey); !kind.Valid() {
		i.stop(kindErr(offset, kind))

		return false
	}
	i.ikey, i.value = ikey, value

	return true
}

func (i *TableIter) load(at int) bool {
	entry := i.table.index[at]
	if cap(i.buf) < entry.length {
		i.buf = make([]byte, entry.length)
	}
	payload, err := i.table.readBlock(entry, i.buf[:entry.length])
	if err != nil {
		i.stop(err)

		return false
	}
	i.at = at
	i.block = newBlockIter(payload)

	return true
}

func (i *TableIter) restart() {
	i.fail = nil
	i.invalidate()
}

func (i *TableIter) stop(err error) {
	i.invalidate()
	i.fail = err
}

func (i *TableIter) invalidate() {
	i.ikey = nil
	i.value = nil
}
