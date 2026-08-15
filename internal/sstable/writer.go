package sstable

import (
	"bytes"
	"fmt"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
)

const (
	_defaultBlockSize       = 4 << 10
	_defaultBloomBitsPerKey = 10
)

type WriterOptions struct {
	BlockSize       int
	BloomBitsPerKey int
}

type Writer struct {
	file       env.File
	blockSize  int
	bitsPerKey int

	block  *blockBuilder
	index  *blockBuilder
	handle []byte
	hashes []uint64

	offset int64
	meta   Meta
	done   bool
}

func NewWriter(f env.File, opts WriterOptions) *Writer {
	blockSize := opts.BlockSize
	if blockSize <= 0 {
		blockSize = _defaultBlockSize
	}
	bitsPerKey := opts.BloomBitsPerKey
	if bitsPerKey <= 0 {
		bitsPerKey = _defaultBloomBitsPerKey
	}

	return &Writer{
		file:       f,
		blockSize:  blockSize,
		bitsPerKey: bitsPerKey,
		block:      newBlockBuilder(),
		index:      newBlockBuilder(),
	}
}

func (w *Writer) Add(ikey, value []byte) error {
	if w.done {
		panic("sstable: Add after Finish")
	}
	if w.meta.EntryCount > 0 && keys.Compare(ikey, w.meta.Largest) <= 0 {
		panic("sstable: keys added out of order")
	}

	if w.meta.EntryCount == 0 {
		w.meta.Smallest = bytes.Clone(ikey)
	}
	w.meta.Largest = append(w.meta.Largest[:0], ikey...)
	w.meta.EntryCount++
	w.hashes = append(w.hashes, filterHash(keys.UserKey(ikey)))

	w.block.add(ikey, value)
	if w.block.size() < w.blockSize {
		return nil
	}

	return w.flushBlock()
}

func (w *Writer) Size() int64 {
	return w.offset
}

func (w *Writer) Finish() (Meta, error) {
	if w.done {
		panic("sstable: Finish after Finish")
	}
	if w.meta.EntryCount == 0 {
		panic("sstable: Finish on an empty table")
	}
	w.done = true

	if !w.block.empty() {
		if err := w.flushBlock(); err != nil {
			return Meta{}, err
		}
	}

	filterOffset := w.offset
	filter := sealBlock(buildFilter(w.hashes, w.bitsPerKey))
	if err := w.write(filter); err != nil {
		return Meta{}, err
	}

	indexOffset := w.offset
	index := sealBlock(w.index.finish())
	if err := w.write(index); err != nil {
		return Meta{}, err
	}

	var raw [_footerSize]byte
	encodeFooter(raw[:], footer{
		indexOffset:  uint64(indexOffset),
		indexLength:  uint64(len(index)),
		filterOffset: uint64(filterOffset),
		filterLength: uint64(len(filter)),
	})
	if err := w.write(raw[:]); err != nil {
		return Meta{}, err
	}

	w.meta.Size = w.offset

	return w.meta, nil
}

func (w *Writer) flushBlock() error {
	raw := sealBlock(w.block.finish())
	offset := w.offset
	if err := w.write(raw); err != nil {
		return err
	}

	w.handle = appendHandle(w.handle[:0], uint64(offset), uint64(len(raw)))
	w.index.add(w.block.lastKey(), w.handle)
	w.block.reset()

	return nil
}

func (w *Writer) write(raw []byte) error {
	if _, err := w.file.Write(raw); err != nil {
		return fmt.Errorf("sstable: write %d bytes at offset %d: %w", len(raw), w.offset, err)
	}
	w.offset += int64(len(raw))

	return nil
}
