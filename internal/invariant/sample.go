package invariant

import "bytes"

const (
	_sampleHead   = 512
	_sampleMiddle = 16
)

type sampler struct {
	stride uint64
	seen   uint64
	head   [][]byte
	last   []byte
}

func newSampler(entryCount uint64) sampler {
	return sampler{stride: max(1, entryCount/_sampleMiddle)}
}

func (s *sampler) add(user []byte) {
	if s.seen < _sampleHead || s.seen%s.stride == 0 {
		s.head = append(s.head, bytes.Clone(user))
	}
	s.last = append(s.last[:0], user...)
	s.seen++
}

func (s *sampler) sampled() [][]byte {
	if len(s.last) == 0 {
		return s.head
	}

	return append(s.head, s.last)
}
