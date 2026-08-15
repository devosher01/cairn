package wal_test

import (
	"math/rand/v2"
	"testing"

	"github.com/devosher01/cairn/internal/wal"
)

const (
	_fuzzNoiseLen  = 64
	_fuzzTailCut   = 3
	_fuzzNoiseSeed = 0x2545F4914F6CDD1D
)

func FuzzReplay(f *testing.F) {
	valid := seedWAL(f, []byte("alpha"), []byte("bravo"), []byte("charlie"))

	f.Add(valid)
	f.Add([]byte{})
	f.Add(valid[:len(valid)-_fuzzTailCut])
	f.Add(noiseBytes(_fuzzNoiseLen))

	f.Fuzz(func(t *testing.T, data []byte) {
		payloads, offset, err := replayAll(data)
		if offset < 0 || offset > int64(len(data)) {
			t.Fatalf("replay of %d bytes stopped at offset %d, want it inside [0,%d]",
				len(data), offset, len(data))
		}

		again, againOffset, againErr := replayAll(data)
		if !equalPayloads(payloads, again) {
			t.Fatalf("two replays of the same %d bytes applied %d then %d payloads",
				len(data), len(payloads), len(again))
		}
		if againOffset != offset {
			t.Fatalf("two replays of the same %d bytes stopped at offset %d then %d",
				len(data), offset, againOffset)
		}
		if errorText(againErr) != errorText(err) {
			t.Fatalf("two replays of the same %d bytes returned error %v then %v",
				len(data), err, againErr)
		}
	})
}

func seedWAL(f *testing.F, payloads ...[]byte) []byte {
	f.Helper()

	file := &fakeFile{}
	w, err := wal.NewWriter(file)
	if err != nil {
		f.Fatalf("NewWriter returned error: %v", err)
	}
	for _, payload := range payloads {
		if err := w.Append(payload); err != nil {
			f.Fatalf("Append returned error: %v", err)
		}
	}

	return file.data
}

func noiseBytes(n int) []byte {
	rng := rand.New(rand.NewPCG(_fuzzNoiseSeed, _fuzzNoiseSeed>>1))
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(rng.Uint64())
	}

	return out
}

func errorText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
