package cairn_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/devosher01/cairn"
)

const (
	_alwaysFirstSeed uint64 = 0
	_offFirstSeed    uint64 = 1000
)

const (
	_alwaysSeeds = 180
	_alwaysShort = 36
	_offSeeds    = 100
	_offShort    = 20
)

func TestModel_SyncAlways(t *testing.T) {
	t.Parallel()

	runCampaign(t, cairn.SyncAlways, _alwaysFirstSeed, campaignSeeds(_alwaysSeeds, _alwaysShort))
}

func TestModel_SyncOff(t *testing.T) {
	t.Parallel()

	runCampaign(t, cairn.SyncOff, _offFirstSeed, campaignSeeds(_offSeeds, _offShort))
}

func runCampaign(t *testing.T, mode cairn.SyncMode, firstSeed uint64, seeds int) {
	t.Helper()

	var total atomic.Int64
	t.Cleanup(func() {
		t.Logf("%d sequences, %d operations executed", seeds, total.Load())
	})

	for i := range seeds {
		seed := firstSeed + uint64(i)
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			t.Parallel()

			total.Add(int64(runSequence(t, seed, mode)))
		})
	}
}

func campaignSeeds(full, short int) int {
	if testing.Short() {
		return short
	}

	return full
}
