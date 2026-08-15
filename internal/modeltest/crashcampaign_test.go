package modeltest_test

import (
	"fmt"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/crashtest"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/invariant"
)

const (
	_campaignAlwaysSeed uint64 = 7100
	_campaignOffSeed    uint64 = 7200
)

const (
	_campaignTornLimit   = 128
	_campaignTornStride  = 512
	_campaignScatter     = 1
	_campaignShortStride = 4
)

func TestCrashCampaign_FullEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		giveMode cairn.SyncMode
		giveSeed uint64
	}{
		{
			name:     "sync always recovers every acknowledged write",
			giveMode: cairn.SyncAlways,
			giveSeed: _campaignAlwaysSeed,
		},
		{
			name:     "sync off recovers an acknowledged prefix",
			giveMode: cairn.SyncOff,
			giveSeed: _campaignOffSeed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for s := range campaignScale() {
				seed := tt.giveSeed + uint64(s)
				t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
					t.Parallel()

					w := runCrashWorkload(t, seed, tt.giveMode)
					ops := w.sim.Ops()
					points := 0
					for point := range crashtest.Points(ops, crashtest.Options{
						TornByteLimit:  _campaignTornLimit,
						TornStride:     _campaignTornStride,
						ScatterSamples: _campaignScatter,
					}) {
						if skipCrashPoint(point) {
							continue
						}
						w.verifyCrashPoint(point)
						points++
					}

					t.Logf("%s seed %d: %d operations, %d acknowledged mutations, %d crash points verified",
						campaignSyncName(tt.giveMode), seed, len(ops), w.oracle.acked(), points)
				})
			}
		})
	}
}

func (w *crashWorkload) verifyCrashPoint(point simenv.CrashPoint) {
	label := crashPointLabel(w.seed, w.mode, point)
	disk := w.sim.MaterializeCrash(point)

	if err := invariant.CheckCrashDisk(disk); err != nil {
		w.t.Fatalf("%s: CheckCrashDisk returned error: %v", label, err)
	}

	sandbox := env.Env{FS: disk, Clock: w.sandbox.Clock, Rand: w.sandbox.Rand}
	db, err := cairn.Open(_dbDir, campaignOptions(sandbox, w.mode))
	if err != nil {
		w.t.Fatalf("%s: Open returned error: %v", label, err)
	}

	dump := crashDump(w.t, db, label)
	if err := db.Close(); err != nil {
		w.t.Fatalf("%s: Close returned error: %v", label, err)
	}
	if err := invariant.Check(disk); err != nil {
		w.t.Fatalf("%s: Check after recovery returned error: %v", label, err)
	}

	w.verifyDurability(label, point, dump)
}

func (w *crashWorkload) verifyDurability(label string, point simenv.CrashPoint, dump map[string][]byte) {
	least := w.durableAt(point.Op)
	if _, ok := w.prefixIndex(dump, least); ok {
		return
	}

	w.t.Fatalf("%s: recovered %s, which is no acknowledged prefix at or beyond mutation %d of %d;"+
		" mutation %d holds %s and the full history holds %s",
		label, formatState(dump), least, w.oracle.acked(), least,
		formatState(w.states[least]), formatState(w.oracle.state))
}

func skipCrashPoint(point simenv.CrashPoint) bool {
	return testing.Short() && point.Op%_campaignShortStride != 0
}

func crashPointLabel(seed uint64, mode cairn.SyncMode, point simenv.CrashPoint) string {
	return fmt.Sprintf("seed %d %s %s crash at op %d torn %d",
		seed, campaignSyncName(mode), crashModeName(point.Mode), point.Op, point.Torn)
}
