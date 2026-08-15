package wal_test

import (
	"bytes"
	"encoding/binary"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/devosher01/cairn/internal/crashtest"
	"github.com/devosher01/cairn/internal/env/simenv"
)

func TestCrashCampaign_ReplayYieldsAnAcknowledgedPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give workload
	}{
		{
			name: "sync always small payloads",
			give: workload{
				seed:         0,
				payloadSizes: cycleSizes(40, 20, 30, 40, 50, 60, 70, 80),
				syncEvery:    1,
			},
		},
		{
			name: "sync never",
			give: workload{
				seed:         1,
				payloadSizes: slices.Repeat([]int{40}, 30),
			},
		},
		{
			name: "sync every third",
			give: workload{
				seed:         2,
				payloadSizes: slices.Repeat([]int{40}, 30),
				syncEvery:    3,
			},
		},
		{
			name: "sync always with rotations",
			give: workload{
				seed:         3,
				payloadSizes: slices.Repeat([]int{50}, 36),
				syncEvery:    1,
				rotations:    []int{11, 23},
			},
		},
		{
			name: "sync every second crossing sectors",
			give: workload{
				seed:         4,
				payloadSizes: slices.Repeat([]int{600}, 24),
				syncEvery:    2,
				rotations:    []int{11},
			},
		},
		{
			name: "large payloads torn at stride",
			give: workload{
				seed:         5,
				payloadSizes: slices.Repeat([]int{8 << 10}, 8),
				syncEvery:    1,
			},
		},
	}

	var total atomic.Int64
	t.Cleanup(func() {
		t.Logf("campaign total: %d crash points, %d materializations", total.Load(), total.Load())
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			total.Add(int64(enumerateCrashes(t, runWorkload(t, tt.give))))
		})
	}
}

func TestCrashCampaign_ShortWriteNeverReplays(t *testing.T) {
	t.Parallel()

	give := workload{
		seed:            6,
		payloadSizes:    slices.Repeat([]int{40}, 20),
		syncEvery:       2,
		shortWriteAt:    12,
		shortWriteBytes: 20,
	}

	r := runWorkload(t, give)

	failed := slices.ContainsFunc(r.sim.Ops(), func(op simenv.Op) bool {
		return op.Kind == simenv.OpWrite && op.Failed
	})
	if !failed {
		t.Fatal("the disk budget did not truncate any write")
	}
	if got := len(r.payloads); got != give.shortWriteAt {
		t.Fatalf("workload appended %d records, want it to stop at %d", got, give.shortWriteAt)
	}

	enumerateCrashes(t, r)
}

func enumerateCrashes(t *testing.T, r *walRun) int {
	t.Helper()

	points := 0
	for point := range crashtest.Points(r.sim.Ops(), crashtest.Options{ScatterSamples: 2}) {
		checkCrashPoint(t, r, point)
		points++
	}
	t.Logf("%d crash points, %d materializations over %d ops and %d appends",
		points, points, len(r.sim.Ops()), len(r.payloads))

	return points
}

func checkCrashPoint(t *testing.T, r *walRun, point simenv.CrashPoint) {
	t.Helper()

	at := describePoint(point)
	got := recoverDisk(t, r.sim.MaterializeCrash(point), at)

	if len(got) > len(r.payloads) {
		t.Fatalf("%s: replayed %d records, want at most the %d appended", at, len(got), len(r.payloads))
	}
	for i, payload := range got {
		if len(payload) < _indexSize {
			t.Fatalf("%s: record %d is %d bytes, want at least %d", at, i, len(payload), _indexSize)
		}
		if index := binary.LittleEndian.Uint32(payload[:_indexSize]); index != uint32(i) {
			t.Fatalf("%s: record %d carries append index %d, want %d", at, i, index, i)
		}
		if !bytes.Equal(payload, r.payloads[i]) {
			t.Fatalf("%s: record %d replayed %d bytes that differ from the %d appended for index %d",
				at, i, len(payload), len(r.payloads[i]), i)
		}
	}

	if lower := ackedCount(t, r, point.Op); len(got) < lower {
		t.Fatalf("%s: replayed %d records, want at least the %d acknowledged as durable", at, len(got), lower)
	}
	if upper := appendedCount(r, point.Op); len(got) > upper {
		t.Fatalf("%s: replayed %d records, want at most the %d appended before the crash", at, len(got), upper)
	}
}

func ackedCount(t *testing.T, r *walRun, at int) int {
	t.Helper()

	count := 0
	broken := false
	for j := range r.payloads {
		durable := r.ackOp[j] != _neverAcked && r.ackOp[j] <= at && r.fileReadyOp[r.appendFile[j]] <= at
		if !durable {
			broken = true

			continue
		}
		if broken {
			t.Fatalf("append %d is durable at op %d after a gap: the acked set is not a prefix", j, at)
		}
		count++
	}

	return count
}

func appendedCount(r *walRun, at int) int {
	count := 0
	for j := range r.payloads {
		if r.writeDoneOp[j] <= at {
			count++
		}
	}

	return count
}
