package sstable

import (
	"bytes"
	"fmt"
	"testing"
)

func TestBuildFilter_HasNoFalseNegatives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		giveBitsPerKey int
	}{
		{
			name:           "one bit per key",
			giveBitsPerKey: 1,
		},
		{
			name:           "four bits per key",
			giveBitsPerKey: 4,
		},
		{
			name:           "default ten bits per key",
			giveBitsPerKey: 10,
		},
		{
			name:           "twenty bits per key",
			giveBitsPerKey: 20,
		},
		{
			name:           "bits per key beyond the probe cap",
			giveBitsPerKey: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hashes := presentHashes()
			filter := buildFilter(hashes, tt.giveBitsPerKey)

			for i, h := range hashes {
				if !filterContains(filter, h) {
					t.Errorf("filterContains(filter, hash of key %d) = false, want true", i)
				}
			}
		})
	}
}

func TestFilterContains_KeepsFalsePositiveRateWithinBudget(t *testing.T) {
	t.Parallel()

	const (
		probes  = 10000
		maxRate = 0.025
	)

	filter := buildFilter(presentHashes(), 10)

	positives := 0
	for i := range probes {
		if filterContains(filter, filterHash(fmt.Appendf(nil, "absent-%06d", i))) {
			positives++
		}
	}

	if rate := float64(positives) / probes; rate > maxRate {
		t.Errorf("false positive rate = %.4f over %d probes, want at most %.4f", rate, probes, maxRate)
	}
}

func TestFilterContains_FailsOpenOnUnusableFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []byte
	}{
		{
			name: "nil filter",
			give: nil,
		},
		{
			name: "empty filter",
			give: []byte{},
		},
		{
			name: "probe count without a bit array",
			give: []byte{7},
		},
		{
			name: "zero probe count",
			give: []byte{0, 0x00, 0x00},
		},
		{
			name: "probe count above the cap",
			give: []byte{31, 0x00, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, key := range []string{"", "a", "hello, world"} {
				if !filterContains(tt.give, filterHash([]byte(key))) {
					t.Errorf("filterContains(%x, hash of %q) = false, want true", tt.give, key)
				}
			}
		})
	}
}

func TestBuildFilter_ReturnsNilWithoutHashes(t *testing.T) {
	t.Parallel()

	if got := buildFilter(nil, 10); got != nil {
		t.Errorf("buildFilter(nil, 10) = %x, want nil", got)
	}
	if got := buildFilter([]uint64{}, 10); got != nil {
		t.Errorf("buildFilter([], 10) = %x, want nil", got)
	}
}

func TestBuildFilter_MatchesGoldenVector(t *testing.T) {
	t.Parallel()

	give := []uint64{0x0102030405060708, 0xdeadbeefcafebabe, 0x0000000100000002}
	want := []byte{0x07, 0xfd, 0x3b, 0x11, 0x55}

	got := buildFilter(give, 10)
	if !bytes.Equal(got, want) {
		t.Fatalf("buildFilter(golden hashes, 10) = %x, want %x", got, want)
	}

	for _, h := range give {
		if !filterContains(got, h) {
			t.Errorf("filterContains(golden filter, 0x%016x) = false, want true", h)
		}
	}
}

func TestBuildFilter_SizesThePayloadFromBitsPerKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		giveHashes     int
		giveBitsPerKey int
		wantK          byte
		wantLen        int
	}{
		{
			name:           "default ten bits per key",
			giveHashes:     3,
			giveBitsPerKey: 10,
			wantK:          7,
			wantLen:        5,
		},
		{
			name:           "one bit per key still probes once",
			giveHashes:     8,
			giveBitsPerKey: 1,
			wantK:          1,
			wantLen:        2,
		},
		{
			name:           "bit array is rounded up to whole bytes",
			giveHashes:     10,
			giveBitsPerKey: 10,
			wantK:          7,
			wantLen:        14,
		},
		{
			name:           "probe count is capped at thirty",
			giveHashes:     1,
			giveBitsPerKey: 100,
			wantK:          30,
			wantLen:        14,
		},
		{
			name:           "bit array is never empty",
			giveHashes:     1,
			giveBitsPerKey: 0,
			wantK:          1,
			wantLen:        2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hashes := make([]uint64, 0, tt.giveHashes)
			for i := range tt.giveHashes {
				hashes = append(hashes, filterHash(fmt.Appendf(nil, "key-%06d", i)))
			}

			got := buildFilter(hashes, tt.giveBitsPerKey)
			if len(got) != tt.wantLen {
				t.Fatalf("len(buildFilter(%d hashes, %d)) = %d, want %d",
					tt.giveHashes, tt.giveBitsPerKey, len(got), tt.wantLen)
			}
			if got[0] != tt.wantK {
				t.Errorf("probe count = %d, want %d", got[0], tt.wantK)
			}
		})
	}
}

func presentHashes() []uint64 {
	const keyCount = 1000

	hashes := make([]uint64, 0, keyCount)
	for i := range keyCount {
		hashes = append(hashes, filterHash(fmt.Appendf(nil, "key-%06d", i)))
	}

	return hashes
}
