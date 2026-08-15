package cairn

import "github.com/devosher01/cairn/internal/engine"

type Batch struct {
	inner *engine.Batch
}

func NewBatch() *Batch {
	return &Batch{inner: engine.NewBatch()}
}

func (b *Batch) Put(key, value []byte) {
	b.inner.Put(key, value)
}

func (b *Batch) Delete(key []byte) {
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
}
