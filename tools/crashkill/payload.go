package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	_keyFormat = "k%08d"
	_keyPrefix = "k"
	_keyDigits = 8
	_valueSize = 64
)

const (
	_valueSeed uint64 = 0xC2B2AE3D27D4EB4F
	_mixGamma  uint64 = 0x9E3779B97F4A7C15
	_mixA      uint64 = 0xBF58476D1CE4E5B9
	_mixB      uint64 = 0x94D049BB133111EB
)

func keyAt(i int) string {
	return fmt.Sprintf(_keyFormat, i)
}

func valueAt(i int) []byte {
	state := mix(_valueSeed ^ uint64(i))
	value := make([]byte, _valueSize)
	for j := range value {
		state = mix(state)
		value[j] = byte(state >> 33)
	}

	return value
}

func indexAt(key string) (int, error) {
	digits, ok := strings.CutPrefix(key, _keyPrefix)
	if !ok || len(digits) != _keyDigits {
		return 0, fmt.Errorf("acked key %q does not match %s", key, _keyFormat)
	}
	i, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("acked key %q: %w", key, err)
	}

	return i, nil
}

func mix(x uint64) uint64 {
	x += _mixGamma
	x = (x ^ (x >> 30)) * _mixA
	x = (x ^ (x >> 27)) * _mixB

	return x ^ (x >> 31)
}
