package cairn

import "github.com/devosher01/cairn/internal/batch"

const (
	MaxBatchLen   = 64 << 20
	MaxBatchCount = 1_000_000
)

type Batch struct {
	inner *batch.Batch
	err   error
}

func NewBatch() *Batch {
	return &Batch{inner: batch.New()}
}

func (b *Batch) Put(key, value []byte) {
	if b.err != nil {
		return
	}
	if err := validateKey(key); err != nil {
		b.err = err
		return
	}
	if err := validateValue(value); err != nil {
		b.err = err
		return
	}
	b.inner.Put(key, value)
}

func (b *Batch) Delete(key []byte) {
	if b.err != nil {
		return
	}
	if err := validateKey(key); err != nil {
		b.err = err
		return
	}
	b.inner.Delete(key)
}

func (b *Batch) Count() uint32 {
	return b.inner.Count()
}

func (b *Batch) Len() int {
	return b.inner.Len()
}

func (b *Batch) Reset() {
	b.inner.Reset()
	b.err = nil
}

func (db *DB) Write(b *Batch) error {
	if b.err != nil {
		return b.err
	}
	if b.inner.Len() > MaxBatchLen || b.inner.Count() > MaxBatchCount {
		return ErrBatchTooLarge
	}
	db.commitMu.Lock()
	defer db.commitMu.Unlock()
	return db.commitLocked(b.inner)
}
