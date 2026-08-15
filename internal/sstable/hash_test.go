package sstable

import "testing"

func TestFilterHash_MatchesMurmur3Vectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want uint64
	}{
		{
			name: "empty input",
			give: "",
			want: 0x0000000000000000,
		},
		{
			name: "single byte tail",
			give: "a",
			want: 0x85555565f6597889,
		},
		{
			name: "three byte tail",
			give: "abc",
			want: 0xb4963f3f3fad7867,
		},
		{
			name: "twelve byte tail",
			give: "hello, world",
			want: 0x342fac623a5ebc8e,
		},
		{
			name: "nine byte tail reaches the second half",
			give: "abcdefghi",
			want: 0x0547c0cff13c7964,
		},
		{
			name: "fifteen byte tail fills every case",
			give: "abcdefghijklmno",
			want: 0x8abe2451890c2ffb,
		},
		{
			name: "one full block without tail",
			give: "0123456789abcdef",
			want: 0x4be06d94cf4ad1a7,
		},
		{
			name: "two full blocks plus a one byte tail",
			give: "The quick brown fox jumps over!!!",
			want: 0x4d0291b492e2c2a4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := filterHash([]byte(tt.give)); got != tt.want {
				t.Errorf("filterHash(%q) = 0x%016x, want 0x%016x", tt.give, got, tt.want)
			}
		})
	}
}

func TestFilterHash_TreatsNilAsEmpty(t *testing.T) {
	t.Parallel()

	if got := filterHash(nil); got != 0 {
		t.Errorf("filterHash(nil) = 0x%016x, want 0x%016x", got, uint64(0))
	}
}

func TestFilterHash_SpreadsAcrossKeys(t *testing.T) {
	t.Parallel()

	const keyCount = 4096

	seen := make(map[uint64]int, keyCount)
	for i := range keyCount {
		key := []byte{byte(i), byte(i >> 8), 'k'}
		h := filterHash(key)
		if previous, ok := seen[h]; ok {
			t.Errorf("filterHash collided on keys %d and %d at 0x%016x", previous, i, h)
		}
		seen[h] = i
	}
}
