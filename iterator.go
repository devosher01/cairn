package cairn

import "github.com/devosher01/cairn/internal/engine"

type IterOptions struct {
	LowerBound []byte
	UpperBound []byte
}

type Iterator struct {
	it *engine.Iterator
}

func (it *Iterator) First() bool {
	return it.it.First()
}

func (it *Iterator) SeekGE(key []byte) bool {
	return it.it.SeekGE(key)
}

func (it *Iterator) Next() bool {
	return it.it.Next()
}

func (it *Iterator) Valid() bool {
	return it.it.Valid()
}

func (it *Iterator) Key() []byte {
	return it.it.Key()
}

func (it *Iterator) Value() []byte {
	return it.it.Value()
}

func (it *Iterator) Error() error {
	return it.it.Error()
}

func (it *Iterator) Close() error {
	return it.it.Close()
}
