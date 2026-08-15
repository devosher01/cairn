package sstable

import (
	"encoding/binary"
	"math/bits"
)

const (
	_murmurC1        = 0x87c37b91114253d5
	_murmurC2        = 0x4cf5ad432745937f
	_murmurBlockSize = 16
	_murmurHalfBlock = _murmurBlockSize / 2
)

func filterHash(key []byte) uint64 {
	var h1, h2 uint64

	blocks := len(key) / _murmurBlockSize
	for i := range blocks {
		base := i * _murmurBlockSize
		k1 := binary.LittleEndian.Uint64(key[base:])
		k2 := binary.LittleEndian.Uint64(key[base+_murmurHalfBlock:])

		k1 *= _murmurC1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= _murmurC2
		h1 ^= k1

		h1 = bits.RotateLeft64(h1, 27)
		h1 += h2
		h1 = h1*5 + 0x52dce729

		k2 *= _murmurC2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= _murmurC1
		h2 ^= k2

		h2 = bits.RotateLeft64(h2, 31)
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}

	tail := key[blocks*_murmurBlockSize:]

	var k1, k2 uint64
	switch len(tail) {
	case 15:
		k2 ^= uint64(tail[14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(tail[13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(tail[12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(tail[11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(tail[10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(tail[9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(tail[8])

		k2 *= _murmurC2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= _murmurC1
		h2 ^= k2

		fallthrough
	case 8:
		k1 ^= uint64(tail[7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(tail[6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(tail[5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(tail[4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(tail[3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(tail[0])

		k1 *= _murmurC1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= _murmurC2
		h1 ^= k1
	}

	h1 ^= uint64(len(key))
	h2 ^= uint64(len(key))

	h1 += h2
	h2 += h1

	h1 = fmix64(h1)
	h2 = fmix64(h2)

	return h1 + h2
}

func fmix64(k uint64) uint64 {
	k ^= k >> 33
	k *= 0xff51afd7ed558ccd
	k ^= k >> 33
	k *= 0xc4ceb9fe1a85ec53
	k ^= k >> 33

	return k
}
