package simenv

import (
	"math/rand/v2"
	"sync"

	"github.com/devosher01/cairn/internal/env"
)

const _streamSalt uint64 = 0x9E3779B97F4A7C15

type simRand struct {
	mu *sync.Mutex
	r  *rand.Rand
}

var _ env.Rand = (*simRand)(nil)

func newRand(mu *sync.Mutex, seed uint64) *simRand {
	return &simRand{mu: mu, r: rand.New(rand.NewPCG(seed, seed^_streamSalt))}
}

func (r *simRand) Uint64() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.r.Uint64()
}
