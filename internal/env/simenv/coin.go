package simenv

import "hash/fnv"

const (
	_mixGamma   uint64 = 0x9E3779B97F4A7C15
	_mixA       uint64 = 0xBF58476D1CE4E5B9
	_mixB       uint64 = 0x94D049BB133111EB
	_dirDomain  uint64 = 0x5DE41F7B2C930A61
	_fileDomain uint64 = 0xA3C59AC3C0FFEE11
)

func coinDir(seed uint64, opIndex int) bool {
	return coin(seed^_dirDomain, uint64(opIndex))
}

func coinSector(seed uint64, name string, sector int) bool {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return coin(seed^_fileDomain^h.Sum64(), uint64(sector))
}

func coin(seed, n uint64) bool {
	return mix(mix(seed)^mix(n))&1 == 1
}

func mix(x uint64) uint64 {
	x += _mixGamma
	x = (x ^ (x >> 30)) * _mixA
	x = (x ^ (x >> 27)) * _mixB
	return x ^ (x >> 31)
}
