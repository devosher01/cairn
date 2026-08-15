package simenv_test

import (
	"testing"

	"github.com/devosher01/cairn/internal/env/simenv"
)

const (
	_pinnedFirst  uint64 = 2850886028085987978
	_pinnedSecond uint64 = 9339104748150398247
)

func TestRand_SameSeedYieldsTheSameSequence(t *testing.T) {
	t.Parallel()

	first := simenv.New(1234).Env().Rand
	second := simenv.New(1234).Env().Rand

	for i := range 8 {
		if got, want := first.Uint64(), second.Uint64(); got != want {
			t.Fatalf("value %d = %d, want %d", i, got, want)
		}
	}
}

func TestRand_SeedPinsTheFirstValues(t *testing.T) {
	t.Parallel()

	want := []uint64{_pinnedFirst, _pinnedSecond}
	rand := simenv.New(1234).Env().Rand

	for i, w := range want {
		if got := rand.Uint64(); got != w {
			t.Errorf("value %d = %d, want %d", i, got, w)
		}
	}
}

func TestRand_DifferentSeedsDiverge(t *testing.T) {
	t.Parallel()

	if simenv.New(1).Env().Rand.Uint64() == simenv.New(2).Env().Rand.Uint64() {
		t.Error("seeds 1 and 2 produced the same first value")
	}
}
