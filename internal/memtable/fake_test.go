package memtable_test

import "github.com/devosher01/cairn/internal/env"

var _ env.Rand = (*fakeRand)(nil)

type fakeRand struct {
	state uint64
}

func newFakeRand(seed uint64) *fakeRand {
	return &fakeRand{state: seed}
}

func (r *fakeRand) Uint64() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB

	return z ^ (z >> 31)
}
