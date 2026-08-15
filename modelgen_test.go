package cairn_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
)

type opKind uint8

const (
	opPut opKind = iota + 1
	opDelete
	opGet
	opScan
	opBatch
	opReopen
	opCrash
	opFlush
	opCompact
	opSnapCreate
	opSnapGet
	opSnapScan
	opSnapClose
)

const (
	_sequenceLength  = 150
	_maxValueLen     = 120
	_shareTotal      = 100
	_putShare        = 30
	_deleteShare     = 42
	_getShare        = 60
	_scanShare       = 68
	_batchShare      = 74
	_reopenShare     = 79
	_snapGetShare    = 83
	_crashShare      = 87
	_flushShare      = 90
	_snapCreateShare = 93
	_snapScanShare   = 96
	_compactShare    = 98
)

const (
	_unboundedShare  = 4
	_minBatchOps     = 2
	_maxBatchOps     = 5
	_batchPutShare   = 3
	_batchShareTotal = 4
)

const (
	_genStream uint64 = 0x9E3779B97F4A7C15
	_mixGamma  uint64 = 0x9E3779B97F4A7C15
	_mixA      uint64 = 0xBF58476D1CE4E5B9
	_mixB      uint64 = 0x94D049BB133111EB
)

type op struct {
	kind      opKind
	key       string
	value     []byte
	lo        []byte
	hi        []byte
	mutations []histOp
}

func (o op) String() string {
	switch o.kind {
	case opPut:
		return fmt.Sprintf("put %s %d bytes", o.key, len(o.value))
	case opDelete:
		return "delete " + o.key
	case opGet:
		return "get " + o.key
	case opScan:
		return fmt.Sprintf("scan [%s,%s) from %s", boundName(o.lo), boundName(o.hi), o.key)
	case opBatch:
		return fmt.Sprintf("batch of %d mutations", len(o.mutations))
	case opReopen:
		return "reopen"
	case opCrash:
		return "crash"
	case opFlush:
		return "flush"
	case opCompact:
		return "compact"
	case opSnapCreate:
		return "snapshot then put " + o.key
	case opSnapGet:
		return "snapshot get " + o.key
	case opSnapScan:
		return "snapshot scan"
	default:
		return "snapshot close"
	}
}

func boundName(bound []byte) string {
	if bound == nil {
		return "*"
	}

	return string(bound)
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
	case roll < _scanShare:
		lo, hi := generateBounds(rng)

		return op{kind: opScan, key: key, lo: lo, hi: hi}
	case roll < _batchShare:
		return op{kind: opBatch, mutations: generateMutations(rng, seed, index)}
	case roll < _reopenShare:
		return op{kind: opReopen}
	case roll < _snapGetShare:
		return op{kind: opSnapGet, key: key}
	case roll < _crashShare:
		return op{kind: opCrash}
	case roll < _flushShare:
		return op{kind: opFlush}
	case roll < _snapCreateShare:
		return op{kind: opSnapCreate, key: key, value: generateValue(rng, seed, index)}
	case roll < _snapScanShare:
		return op{kind: opSnapScan}
	case roll < _compactShare:
		return op{kind: opCompact}
	default:
		return op{kind: opSnapClose}
	}
}

func generateBounds(rng *rand.Rand) ([]byte, []byte) {
	lo, hi := generateBound(rng), generateBound(rng)
	if lo != nil && hi != nil && bytes.Compare(lo, hi) > 0 {
		lo, hi = hi, lo
	}

	return lo, hi
}

func generateBound(rng *rand.Rand) []byte {
	if rng.IntN(_unboundedShare) == 0 {
		return nil
	}

	return []byte(domainKey(rng.IntN(_keyDomain)))
}

func generateMutations(rng *rand.Rand, seed uint64, index int) []histOp {
	out := make([]histOp, _minBatchOps+rng.IntN(_maxBatchOps-_minBatchOps+1))
	for i := range out {
		key := domainKey(rng.IntN(_keyDomain))
		if rng.IntN(_batchShareTotal) < _batchPutShare {
			out[i] = histOp{kind: histPut, key: key, value: generateValue(rng, seed, index)}

			continue
		}
		out[i] = histOp{kind: histDelete, key: key}
	}

	return out
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
