package engine

import (
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/manifest"
	"github.com/devosher01/cairn/internal/sstable"
)

type tableWriter struct {
	db  *DB
	num uint64
	f   env.File
	w   *sstable.Writer
	err error
}

func (db *DB) newTableWriter(num uint64) *tableWriter {
	f, err := db.fs.Create(sstName(num))
	if err != nil {
		return &tableWriter{err: err}
	}
	w := sstable.NewWriter(f, sstable.WriterOptions{
		BlockSize:       db.opts.BlockSize,
		BloomBitsPerKey: db.opts.BloomBitsPerKey,
	})
	return &tableWriter{db: db, num: num, f: f, w: w}
}

func (t *tableWriter) add(ikey, value []byte) error {
	return t.w.Add(ikey, value)
}

func (t *tableWriter) size() int64 {
	return t.w.Size()
}

func (t *tableWriter) finish() (manifest.Table, error) {
	meta, err := t.w.Finish()
	if err != nil {
		t.abort()
		return manifest.Table{}, err
	}
	if err := t.f.Sync(); err != nil {
		t.abort()
		return manifest.Table{}, err
	}
	if err := t.f.Close(); err != nil {
		_ = t.db.fs.Remove(sstName(t.num))
		return manifest.Table{}, err
	}
	return manifest.Table{
		FileNum:    t.num,
		Size:       uint64(meta.Size),
		EntryCount: meta.EntryCount,
		Smallest:   meta.Smallest,
		Largest:    meta.Largest,
	}, nil
}

func (t *tableWriter) abort() {
	if t.f != nil {
		_ = t.f.Close()
		_ = t.db.fs.Remove(sstName(t.num))
	}
}
