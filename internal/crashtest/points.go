package crashtest

import (
	"iter"

	"github.com/devosher01/cairn/internal/env/simenv"
)

func Points(ops []simenv.Op, opts Options) iter.Seq[simenv.CrashPoint] {
	opts = opts.withDefaults()

	return func(yield func(simenv.CrashPoint) bool) {
		for i := range len(ops) + 1 {
			if !yield(simenv.CrashPoint{Op: i, Mode: simenv.CrashNone}) {
				return
			}
			if !yield(simenv.CrashPoint{Op: i, Mode: simenv.CrashPrefix}) {
				return
			}
			for k := range opts.ScatterSamples {
				scatter := simenv.CrashPoint{
					Op:          i,
					Mode:        simenv.CrashScatter,
					ScatterSeed: opts.ScatterSeed + uint64(k),
				}
				if !yield(scatter) {
					return
				}
			}

			if i == len(ops) || !tearable(ops[i]) {
				continue
			}
			for cut := range tornCuts(ops[i].Len, opts) {
				if !yield(simenv.CrashPoint{Op: i, Torn: cut, Mode: simenv.CrashPrefix}) {
					return
				}
				scatter := simenv.CrashPoint{
					Op:          i,
					Torn:        cut,
					Mode:        simenv.CrashScatter,
					ScatterSeed: opts.ScatterSeed,
				}
				if !yield(scatter) {
					return
				}
			}
		}
	}
}

func tearable(op simenv.Op) bool {
	return op.Kind == simenv.OpWrite && !op.Failed && op.Len > 1
}

func tornCuts(length int, opts Options) iter.Seq[int] {
	step := 1
	if length > opts.TornByteLimit {
		step = opts.TornStride
	}
	last := length - 1

	return func(yield func(int) bool) {
		for cut := 1; cut < last; cut += step {
			if !yield(cut) {
				return
			}
		}
		yield(last)
	}
}
