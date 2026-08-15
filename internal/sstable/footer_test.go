package sstable

import (
	"encoding/hex"
	"errors"
	"math"
	"testing"
)

func TestFooter_RoundTripsThroughEncodeAndDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give footer
	}{
		{name: "zero handles"},
		{name: "small handles", give: footer{indexOffset: 1, indexLength: 2, filterOffset: 3, filterLength: 4}},
		{
			name: "handles at the 64 bit limit",
			give: footer{
				indexOffset:  math.MaxUint64,
				indexLength:  math.MaxUint64,
				filterOffset: math.MaxUint64,
				filterLength: math.MaxUint64,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var raw [_footerSize]byte
			encodeFooter(raw[:], tt.give)

			got, err := decodeFooter(raw[:])
			if err != nil {
				t.Fatalf("decodeFooter(%x) = %v, want no error", raw, err)
			}
			if got != tt.give {
				t.Errorf("decodeFooter = %+v, want %+v", got, tt.give)
			}
		})
	}
}

func TestEncodeFooter_MatchesTheFixedVector(t *testing.T) {
	t.Parallel()

	const want = "010000000000000002000000000000000300000000000000040000000000000001000000481f6bc9434149524e535354"

	var raw [_footerSize]byte
	encodeFooter(raw[:], footer{indexOffset: 1, indexLength: 2, filterOffset: 3, filterLength: 4})

	if got := hex.EncodeToString(raw[:]); got != want {
		t.Fatalf("encodeFooter = %s, want %s", got, want)
	}
	if got := string(raw[_footerMagicAt:_footerSize]); got != _footerMagic {
		t.Errorf("magic = %q, want %q", got, _footerMagic)
	}
}

func TestDecodeFooter_RejectsCorruptFooters(t *testing.T) {
	t.Parallel()

	var valid [_footerSize]byte
	encodeFooter(valid[:], footer{indexOffset: 64, indexLength: 32, filterOffset: 16, filterLength: 48})

	tests := []struct {
		name string
		give []byte
	}{
		{name: "zeroed footer", give: make([]byte, _footerSize)},
		{name: "flipped magic byte", give: flipBlockByte(valid[:], _footerMagicAt)},
		{name: "flipped last magic byte", give: flipBlockByte(valid[:], _footerSize-1)},
		{name: "unknown version", give: setFooterVersion(valid[:], _footerVersion+1)},
		{name: "flipped crc byte", give: flipBlockByte(valid[:], _footerCRCAt)},
		{name: "flipped index offset byte", give: flipBlockByte(valid[:], 0)},
		{name: "flipped filter length byte", give: flipBlockByte(valid[:], 24)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeFooter(tt.give)
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("decodeFooter(%x) error = %v, want %v", tt.give, err, ErrCorrupt)
			}
			if got != (footer{}) {
				t.Errorf("decodeFooter = %+v, want the zero footer", got)
			}
		})
	}
}
