package osenv

import (
	"math/rand/v2"

	"github.com/devosher01/cairn/internal/env"
)

type rng struct{}

var _ env.Rand = rng{}

func (rng) Uint64() uint64 {
	return rand.Uint64()
}
