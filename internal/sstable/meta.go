package sstable

type Meta struct {
	Size       int64
	EntryCount uint64
	Smallest   []byte
	Largest    []byte
}
