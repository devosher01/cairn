package cairn

import (
	"container/heap"
	"iter"

	"github.com/devosher01/cairn/internal/keys"
)

type mergeSource struct {
	next func() ([]byte, []byte, bool)
	stop func()
	fail func() error

	ikey  []byte
	value []byte
	order int
}

func newMergeSource(seq iter.Seq2[[]byte, []byte], fail func() error, order int) *mergeSource {
	next, stop := iter.Pull2(seq)
	return &mergeSource{next: next, stop: stop, fail: fail, order: order}
}

func (s *mergeSource) advance() bool {
	ikey, value, ok := s.next()
	if !ok {
		return false
	}
	s.ikey, s.value = ikey, value
	return true
}

type mergeHeap []*mergeSource

func (h mergeHeap) Len() int { return len(h) }

func (h mergeHeap) Less(i, j int) bool {
	if c := keys.Compare(h[i].ikey, h[j].ikey); c != 0 {
		return c < 0
	}
	return h[i].order < h[j].order
}

func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x any) { *h = append(*h, x.(*mergeSource)) }

func (h *mergeHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

type mergeIter struct {
	heap    mergeHeap
	sources []*mergeSource
	emitKey []byte
	emitVal []byte
	prev    []byte
	prevSet bool
}

func newMergeIter(sources []*mergeSource) *mergeIter {
	m := &mergeIter{sources: sources}
	for _, s := range sources {
		if s.advance() {
			m.heap = append(m.heap, s)
		}
	}
	heap.Init(&m.heap)
	return m
}

func (m *mergeIter) next() ([]byte, []byte, bool) {
	for len(m.heap) > 0 {
		top := m.heap[0]
		m.emitKey = append(m.emitKey[:0], top.ikey...)
		m.emitVal = append(m.emitVal[:0], top.value...)
		if top.advance() {
			heap.Fix(&m.heap, 0)
		} else {
			heap.Pop(&m.heap)
		}
		if m.prevSet && keys.Compare(m.emitKey, m.prev) == 0 {
			continue
		}
		m.prev = append(m.prev[:0], m.emitKey...)
		m.prevSet = true
		return m.emitKey, m.emitVal, true
	}
	return nil, nil, false
}

func (m *mergeIter) close() error {
	var first error
	for _, s := range m.sources {
		s.stop()
		if err := s.fail(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
