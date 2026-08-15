package sstable

import "math"

const (
	_filterMaxK       = 30
	_filterHeaderSize = 1
)

func buildFilter(hashes []uint64, bitsPerKey int) []byte {
	if len(hashes) == 0 {
		return nil
	}

	k := max(1, min(_filterMaxK, int(math.Round(float64(bitsPerKey)*math.Ln2))))
	filter := make([]byte, _filterHeaderSize+max(1, (len(hashes)*bitsPerKey+7)/8))
	filter[0] = byte(k)

	bitmap := filter[_filterHeaderSize:]
	m := uint32(len(bitmap) * 8)
	for _, h := range hashes {
		for i := range k {
			pos := filterProbe(h, i, m)
			bitmap[pos/8] |= 1 << (pos % 8)
		}
	}

	return filter
}

func filterContains(filter []byte, h uint64) bool {
	if len(filter) < _filterHeaderSize+1 {
		return true
	}

	k := int(filter[0])
	if k == 0 || k > _filterMaxK {
		return true
	}

	bitmap := filter[_filterHeaderSize:]
	m := uint32(len(bitmap) * 8)
	for i := range k {
		pos := filterProbe(h, i, m)
		if bitmap[pos/8]&(1<<(pos%8)) == 0 {
			return false
		}
	}

	return true
}

func filterProbe(h uint64, i int, m uint32) uint32 {
	return (uint32(h) + uint32(i)*uint32(h>>32)) % m
}
