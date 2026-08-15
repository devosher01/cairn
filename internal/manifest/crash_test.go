package manifest_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/crashtest"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/manifest"
)

type outcome uint8

const (
	_absent outcome = iota
	_installedA
	_installedB
)

func TestInstallCampaign_EveryCrashLoadsOneInstalledState(t *testing.T) {
	t.Parallel()

	sim := simenv.New(0)
	stateA, stateB := campaignStateA(), campaignStateB()
	installOn(t, sim, stateA, stateB)

	ops := sim.Ops()
	durable := slices.IndexFunc(ops, func(op simenv.Op) bool {
		return op.Kind == simenv.OpSyncDir
	})
	if durable < 0 {
		t.Fatal("Install never synced the directory")
	}

	points := 0
	seen := make(map[outcome]int, 3)
	for point := range crashtest.Points(ops, crashtest.Options{ScatterSamples: 2}) {
		seen[checkCrashPoint(t, sim, point, durable, stateA, stateB)]++
		points++
	}
	t.Logf("%d crash points, %d materializations over %d ops: %d absent, %d state A, %d state B",
		points, points, len(ops), seen[_absent], seen[_installedA], seen[_installedB])

	if seen[_absent] == 0 || seen[_installedA] == 0 || seen[_installedB] == 0 {
		t.Errorf("the campaign never reached one of the three outcomes: %v", seen)
	}

	got, exists, err := manifest.Load(sim.Env().FS)
	if err != nil {
		t.Fatalf("Load after the campaign returned error: %v", err)
	}
	if !exists || !reflect.DeepEqual(got, stateB) {
		t.Errorf("Load after the campaign returned (%+v, %t), want the second installed state", got, exists)
	}
}

func checkCrashPoint(t *testing.T, sim *simenv.Sim, point simenv.CrashPoint, durable int, a, b manifest.State) outcome {
	t.Helper()

	at := describePoint(point)
	got, exists, err := manifest.Load(sim.MaterializeCrash(point))
	if err != nil {
		t.Fatalf("%s: Load returned error: %v", at, err)
	}
	if !exists {
		if point.Op > durable {
			t.Fatalf("%s: Load reported no manifest after the first install was durable", at)
		}
		if !reflect.DeepEqual(got, manifest.State{}) {
			t.Errorf("%s: Load returned state %+v with no manifest, want the zero state", at, got)
		}

		return _absent
	}
	if reflect.DeepEqual(got, a) {
		return _installedA
	}
	if reflect.DeepEqual(got, b) {
		return _installedB
	}
	t.Fatalf("%s: Load returned %+v, want either installed state", at, got)

	return _absent
}
