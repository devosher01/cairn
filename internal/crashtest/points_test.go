package crashtest_test

import (
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/crashtest"
	"github.com/devosher01/cairn/internal/env/simenv"
)

func TestPoints_EnumeratesEveryModeAtEveryOpIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		giveOptions crashtest.Options
		wantNone    int
		wantPrefix  int
		wantScatter int
	}{
		{
			name:        "defaults tear every byte of both writes",
			giveOptions: crashtest.Options{},
			wantNone:    _opCount + 1,
			wantPrefix:  _opCount + 1 + 3 + 1499,
			wantScatter: 2*(_opCount+1) + 3 + 1499,
		},
		{
			name:        "one scatter sample per op index",
			giveOptions: crashtest.Options{ScatterSamples: 1},
			wantNone:    _opCount + 1,
			wantPrefix:  _opCount + 1 + 3 + 1499,
			wantScatter: _opCount + 1 + 3 + 1499,
		},
		{
			name:        "three scatter samples per op index",
			giveOptions: crashtest.Options{ScatterSamples: 3},
			wantNone:    _opCount + 1,
			wantPrefix:  _opCount + 1 + 3 + 1499,
			wantScatter: 3*(_opCount+1) + 3 + 1499,
		},
		{
			name:        "stride shortens the torn variants of the large write",
			giveOptions: crashtest.Options{TornByteLimit: 1000, TornStride: 500, ScatterSamples: 1},
			wantNone:    _opCount + 1,
			wantPrefix:  _opCount + 1 + 3 + 4,
			wantScatter: _opCount + 1 + 3 + 4,
		},
	}

	sim := buildSim(t)
	ops := sim.Ops()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			points := collect(ops, tt.giveOptions)

			if got := countMode(points, simenv.CrashNone); got != tt.wantNone {
				t.Errorf("none points = %d, want %d", got, tt.wantNone)
			}
			if got := countMode(points, simenv.CrashPrefix); got != tt.wantPrefix {
				t.Errorf("prefix points = %d, want %d", got, tt.wantPrefix)
			}
			if got := countMode(points, simenv.CrashScatter); got != tt.wantScatter {
				t.Errorf("scatter points = %d, want %d", got, tt.wantScatter)
			}
			if got, want := len(points), tt.wantNone+tt.wantPrefix+tt.wantScatter; got != want {
				t.Errorf("total points = %d, want %d", got, want)
			}
		})
	}
}

func TestPoints_ZeroOptionsEqualTheDocumentedDefaults(t *testing.T) {
	t.Parallel()

	ops := buildSim(t).Ops()

	got := collect(ops, crashtest.Options{})
	want := collect(ops, crashtest.Options{TornByteLimit: 4096, TornStride: 512, ScatterSamples: 2})

	if !slices.Equal(got, want) {
		t.Errorf("zero options yielded %d points, the explicit defaults %d", len(got), len(want))
	}
}

func TestPoints_TornCutsCoverEveryByteOfASmallWrite(t *testing.T) {
	t.Parallel()

	ops := buildSim(t).Ops()
	points := collect(ops, crashtest.Options{})
	want := []int{1, 2, 3}

	for _, mode := range []simenv.CrashMode{simenv.CrashPrefix, simenv.CrashScatter} {
		if got := tornCutsAt(points, _smallWriteOp, mode); !slices.Equal(got, want) {
			t.Errorf("torn cuts for mode %d = %v, want %v", mode, got, want)
		}
	}
	if got := tornCutsAt(points, _smallWriteOp, simenv.CrashNone); got != nil {
		t.Errorf("torn cuts in none mode = %v, want none", got)
	}
}

func TestPoints_TornCutsStrideThroughALargeWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		giveOptions crashtest.Options
		want        []int
	}{
		{
			name:        "byte limit above the write tears every byte",
			giveOptions: crashtest.Options{TornByteLimit: _largeWriteLen},
			want:        slicesRange(1, _largeWriteLen-1),
		},
		{
			name:        "byte limit below the write strides and keeps the last cut",
			giveOptions: crashtest.Options{TornByteLimit: 1000, TornStride: 500},
			want:        []int{1, 501, 1001, 1499},
		},
		{
			name:        "stride larger than the write keeps only the first and last cut",
			giveOptions: crashtest.Options{TornByteLimit: 8, TornStride: 4096},
			want:        []int{1, 1499},
		},
	}

	ops := buildSim(t).Ops()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tornCutsAt(collect(ops, tt.giveOptions), _largeWriteOp, simenv.CrashPrefix)

			if !slices.Equal(got, tt.want) {
				t.Errorf("torn cuts = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPoints_SkipsWritesSimenvRefusesToTear(t *testing.T) {
	t.Parallel()

	sim := buildSim(t)
	ops := sim.Ops()
	points := collect(ops, crashtest.Options{})

	if got := tornCutsAt(points, _failedWriteOp, simenv.CrashPrefix); got != nil {
		t.Errorf("torn cuts for the failed write = %v, want none", got)
	}
	if got := tornCutsAt(points, _tinyWriteOp, simenv.CrashPrefix); got != nil {
		t.Errorf("torn cuts for the single-byte write = %v, want none", got)
	}
	if got := tornCutsAt(points, len(ops), simenv.CrashPrefix); got != nil {
		t.Errorf("torn cuts past the last op = %v, want none", got)
	}

	for _, p := range points {
		if p.Torn == 0 {
			continue
		}
		op := ops[p.Op]
		if op.Kind != simenv.OpWrite || op.Failed || p.Torn < 1 || p.Torn >= op.Len {
			t.Fatalf("point %+v tears op %+v, which simenv rejects", p, op)
		}
	}
}

func TestPoints_EveryPointMaterializes(t *testing.T) {
	t.Parallel()

	sim := buildSim(t)
	opts := crashtest.Options{TornByteLimit: 8, TornStride: 300}

	count := 0
	for p := range crashtest.Points(sim.Ops(), opts) {
		sim.MaterializeCrash(p)
		count++
	}

	if want := 4*(_opCount+1) + 2*(3+6); count != want {
		t.Errorf("materialized %d points, want %d", count, want)
	}
}

func TestPoints_StopsWhenTheCallerBreaks(t *testing.T) {
	t.Parallel()

	ops := buildSim(t).Ops()

	count := 0
	for range crashtest.Points(ops, crashtest.Options{}) {
		count++
		if count == 5 {
			break
		}
	}

	if count != 5 {
		t.Errorf("saw %d points before the break, want 5", count)
	}
}

func TestPoints_ScatterSeedsAdvancePerSample(t *testing.T) {
	t.Parallel()

	ops := buildSim(t).Ops()
	points := collect(ops, crashtest.Options{ScatterSamples: 3, ScatterSeed: 100})

	var base, torn []uint64
	for _, p := range points {
		if p.Mode != simenv.CrashScatter || p.Op != _smallWriteOp {
			continue
		}
		if p.Torn == 0 {
			base = append(base, p.ScatterSeed)
			continue
		}
		torn = append(torn, p.ScatterSeed)
	}

	if want := []uint64{100, 101, 102}; !slices.Equal(base, want) {
		t.Errorf("scatter seeds = %v, want %v", base, want)
	}
	if want := []uint64{100, 100, 100}; !slices.Equal(torn, want) {
		t.Errorf("torn scatter seeds = %v, want %v", torn, want)
	}
}
