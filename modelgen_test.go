package cairn_test

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
)

type opKind uint8

const (
	opPut opKind = iota + 1
	opDelete
	opGet
	opReopen
	opCrash
	opFlush
	opCompact
)

const (
	_sequenceLength = 150
	_maxValueLen    = 120
	_shareTotal     = 100
	_putShare       = 40
	_deleteShare    = 58
	_getShare       = 83
	_reopenShare    = 88
	_crashShare     = 92
	_flushShare     = 97
)

const (
	_genStream uint64 = 0x9E3779B97F4A7C15
	_mixGamma  uint64 = 0x9E3779B97F4A7C15
	_mixA      uint64 = 0xBF58476D1CE4E5B9
	_mixB      uint64 = 0x94D049BB133111EB
)

type op struct {
	kind  opKind
	key   string
	value []byte
}

func (o op) String() string {
	switch o.kind {
	case opPut:
		return fmt.Sprintf("put %s %d bytes", o.key, len(o.value))
	case opDelete:
		return "delete " + o.key
	case opGet:
		return "get " + o.key
	case opReopen:
		return "reopen"
	case opCrash:
		return "crash"
	case opFlush:
		return "flush"
	default:
		return "compact"
	}
}

func generate(seed uint64, count int) []op {
	rng := rand.New(rand.NewPCG(seed, seed^_genStream))
	out := make([]op, count)
	for i := range out {
		out[i] = generateOp(rng, seed, i)
	}

	return out
}

func generateOp(rng *rand.Rand, seed uint64, index int) op {
	key := domainKey(rng.IntN(_keyDomain))
	switch roll := rng.IntN(_shareTotal); {
	case roll < _putShare:
		return op{kind: opPut, key: key, value: generateValue(rng, seed, index)}
	case roll < _deleteShare:
		return op{kind: opDelete, key: key}
	case roll < _getShare:
		return op{kind: opGet, key: key}
	case roll < _reopenShare:
		return op{kind: opReopen}
	case roll < _crashShare:
		return op{kind: opCrash}
	case roll < _flushShare:
		return op{kind: opFlush}
	default:
		return op{kind: opCompact}
	}
}

func generateValue(rng *rand.Rand, seed uint64, index int) []byte {
	value := make([]byte, rng.IntN(_maxValueLen+1))
	if len(value) >= _opIndexBytes {
		binary.BigEndian.PutUint32(value[:_opIndexBytes], uint32(index))
	}
	state := seed ^ uint64(index)
	for i := min(len(value), _opIndexBytes); i < len(value); i++ {
		state = mix(state)
		value[i] = byte(state >> 33)
	}

	return value
}

func mix(x uint64) uint64 {
	x += _mixGamma
	x = (x ^ (x >> 30)) * _mixA
	x = (x ^ (x >> 27)) * _mixB

	return x ^ (x >> 31)
}
