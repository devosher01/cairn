package wal_test

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/wal"
)

const (
	_indexSize     = 4
	_neverAcked    = -1
	_mixMultiplier = 6364136223846793005
	_mixIncrement  = 1442695040888963407
)

type workload struct {
	seed            uint64
	payloadSizes    []int
	syncEvery       int
	rotations       []int
	shortWriteAt    int
	shortWriteBytes int
}

type walRun struct {
	sim         *simenv.Sim
	fsys        env.FS
	payloads    [][]byte
	writeDoneOp []int
	ackOp       []int
	appendFile  []int
	fileReadyOp []int
	pending     []int
}

func runWorkload(t *testing.T, give workload) *walRun {
	t.Helper()

	sim := simenv.New(give.seed)
	r := &walRun{sim: sim, fsys: sim.Env().FS}
	writer := r.openWAL(t)

	for j, size := range give.payloadSizes {
		if give.shortWriteBytes > 0 && j == give.shortWriteAt {
			sim.SetDiskBudget(int64(give.shortWriteBytes))
		}

		payload := makePayload(give.seed, j, size)
		if err := writer.Append(payload); err != nil {
			sim.SetDiskBudget(-1)
			r.closeWAL(t, writer)

			return r
		}
		r.writeDoneOp = append(r.writeDoneOp, len(sim.Ops()))
		r.payloads = append(r.payloads, payload)
		r.ackOp = append(r.ackOp, _neverAcked)
		r.appendFile = append(r.appendFile, len(r.fileReadyOp)-1)
		r.pending = append(r.pending, j)

		if give.syncEvery > 0 && (j+1)%give.syncEvery == 0 {
			if err := writer.Sync(); err != nil {
				t.Fatalf("Sync after append %d returned error: %v", j, err)
			}
			r.ack()
		}
		if slices.Contains(give.rotations, j) {
			r.closeWAL(t, writer)
			writer = r.openWAL(t)
		}
	}
	r.closeWAL(t, writer)

	return r
}

func (r *walRun) openWAL(t *testing.T) *wal.Writer {
	t.Helper()

	name := fmt.Sprintf("%06d.wal", len(r.fileReadyOp)+1)
	f, err := r.fsys.Create(name)
	if err != nil {
		t.Fatalf("Create %s returned error: %v", name, err)
	}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter for %s returned error: %v", name, err)
	}
	if err := r.fsys.SyncDir(); err != nil {
		t.Fatalf("SyncDir after creating %s returned error: %v", name, err)
	}
	r.fileReadyOp = append(r.fileReadyOp, len(r.sim.Ops()))

	return w
}

func (r *walRun) closeWAL(t *testing.T, w *wal.Writer) {
	t.Helper()

	if err := w.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	r.ack()
}

func (r *walRun) ack() {
	at := len(r.sim.Ops())
	for _, j := range r.pending {
		r.ackOp[j] = at
	}
	r.pending = nil
}

func makePayload(seed uint64, index, size int) []byte {
	out := make([]byte, max(size, _indexSize))
	binary.LittleEndian.PutUint32(out[:_indexSize], uint32(index))

	mix := seed*_mixMultiplier + uint64(index)
	for i := _indexSize; i < len(out); i++ {
		mix = mix*_mixMultiplier + _mixIncrement
		out[i] = byte(mix >> 33)
	}

	return out
}

func cycleSizes(count int, sizes ...int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = sizes[i%len(sizes)]
	}

	return out
}
