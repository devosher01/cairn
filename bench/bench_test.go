package bench

import "testing"

const (
	_preloadKeys = 200_000
	_randomSpace = 1 << 20
	_warmReads   = 1000
)

func BenchmarkFillSeq(b *testing.B) {
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			s := openStore(b, e, b.TempDir(), _syncOff)
			runFill(b, s, newValuePool(_seedValues, _valueLen), nil)
		})
	}
}

func BenchmarkFillSync(b *testing.B) {
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			s := openStore(b, e, b.TempDir(), _syncOn)
			runFill(b, s, newValuePool(_seedValues, _valueLen), nil)
		})
	}
}

func BenchmarkFillRandom(b *testing.B) {
	order := randomKeys(_sequenceLen, _randomSpace, _seedFillRand)
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			s := openStore(b, e, b.TempDir(), _syncOff)
			runFill(b, s, newValuePool(_seedValues, _valueLen), order)
		})
	}
}

func runFill(b *testing.B, s store, pool valuePool, order []uint32) {
	b.Helper()
	key := make([]byte, _keyLen)
	var i uint64
	b.ReportAllocs()
	b.SetBytes(_entryBytes)
	if order == nil {
		for b.Loop() {
			fillKey(key, i)
			if err := s.put(key, pool.at(i)); err != nil {
				b.Fatalf("put: %v", err)
			}
			i++
		}
		return
	}
	for b.Loop() {
		n := uint64(order[i&_sequenceMask])
		fillKey(key, n)
		if err := s.put(key, pool.at(n)); err != nil {
			b.Fatalf("put: %v", err)
		}
		i++
	}
}

func BenchmarkReadRandom(b *testing.B) {
	order := randomKeys(_sequenceLen, _preloadKeys, _seedReadRand)
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			dir := b.TempDir()
			pool := newValuePool(_seedValues, _valueLen)
			s := openStore(b, e, dir, _syncOff)
			preload(b, s, pool, _preloadKeys)
			reopenStore(b, s, dir, _syncOff)

			key := make([]byte, _keyLen)
			for i := range uint64(_warmReads) {
				fillKey(key, i)
				if _, err := s.get(key); err != nil {
					b.Fatalf("warm get: %v", err)
				}
			}

			b.ReportAllocs()
			var i uint64
			for b.Loop() {
				fillKey(key, uint64(order[i&_sequenceMask]))
				value, err := s.get(key)
				if err != nil {
					b.Fatalf("get: %v", err)
				}
				if len(value) != _valueLen {
					b.Fatalf("get returned %d bytes, want %d", len(value), _valueLen)
				}
				i++
			}
		})
	}
}

func BenchmarkScan(b *testing.B) {
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			dir := b.TempDir()
			pool := newValuePool(_seedValues, _valueLen)
			s := openStore(b, e, dir, _syncOff)
			preload(b, s, pool, _preloadKeys)
			reopenStore(b, s, dir, _syncOff)
			countScan(b, s)

			b.ReportAllocs()
			b.SetBytes(_entryBytes * _preloadKeys)
			for b.Loop() {
				countScan(b, s)
			}
		})
	}
}

func countScan(b *testing.B, s store) {
	count := 0
	err := s.scan(func(_, _ []byte) {
		count++
	})
	if err != nil {
		b.Fatalf("scan: %v", err)
	}
	if count != _preloadKeys {
		b.Fatalf("scan counted %d entries, want %d", count, _preloadKeys)
	}
}
