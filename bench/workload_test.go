package bench

import (
	"math/rand/v2"
	"testing"
)

const (
	_keyLen     = 16
	_valueLen   = 100
	_entryBytes = _keyLen + _valueLen
)

const (
	_syncOff = false
	_syncOn  = true
)

const (
	_poolBytes    = 1 << 20
	_poolMask     = _poolBytes - 1
	_sequenceLen  = 1 << 20
	_sequenceMask = _sequenceLen - 1
)

const (
	_seedValues    = 0x5EA1ED
	_seedFillRand  = 0xF111A11
	_seedReadRand  = 0x0DDBA11
	_seedAmpOrder  = 0xA11FEED
	_seedAmpValues = 0xB0DEDA7
	_seedAmpReads  = 0xC0FFEE5
)

type engine struct {
	name     string
	newStore func() store
}

func engines() []engine {
	return []engine{
		{name: "cairn", newStore: func() store { return &cairnStore{} }},
		{name: "bolt", newStore: func() store { return &boltStore{} }},
		{name: "pebble", newStore: func() store { return &pebbleStore{} }},
	}
}

func openStore(tb testing.TB, e engine, dir string, sync bool) store {
	s := e.newStore()
	s.open(tb, dir, sync)
	tb.Cleanup(func() {
		if err := s.close(); err != nil {
			tb.Errorf("%s close: %v", e.name, err)
		}
	})
	return s
}

func reopenStore(tb testing.TB, s store, dir string, sync bool) {
	if err := s.close(); err != nil {
		tb.Fatalf("close: %v", err)
	}
	s.open(tb, dir, sync)
}

func preload(tb testing.TB, s store, pool valuePool, count int) {
	key := make([]byte, _keyLen)
	for i := range uint64(count) {
		fillKey(key, i)
		if err := s.put(key, pool.at(i)); err != nil {
			tb.Fatalf("preload put: %v", err)
		}
	}
}

func fillKey(dst []byte, n uint64) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = byte('0' + n%10)
		n /= 10
	}
}

type valuePool struct {
	buf   []byte
	width uint64
}

func newValuePool(seed uint64, width int) valuePool {
	rng := rand.New(rand.NewPCG(seed, seed^_poolBytes))
	buf := make([]byte, _poolBytes+width)
	for i := range buf {
		buf[i] = byte(rng.Uint32())
	}
	return valuePool{buf: buf, width: uint64(width)}
}

func (p valuePool) at(n uint64) []byte {
	off := n & _poolMask
	return p.buf[off : off+p.width]
}

func randomKeys(count int, space uint32, seed uint64) []uint32 {
	rng := rand.New(rand.NewPCG(seed, seed^_sequenceLen))
	out := make([]uint32, count)
	for i := range out {
		out[i] = rng.Uint32N(space)
	}
	return out
}

func shuffledKeys(count int, seed uint64) []uint32 {
	rng := rand.New(rand.NewPCG(seed, seed^_sequenceLen))
	out := make([]uint32, count)
	for i := range out {
		out[i] = uint32(i)
	}
	rng.Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}
