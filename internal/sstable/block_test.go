package sstable

import (
	"bytes"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestBlockBuilder_RoundTripsEntriesThroughSealAndVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []blockEntry
	}{
		{name: "no entries", give: nil},
		{name: "single entry", give: []blockEntry{{ikey: "key", value: "value"}}},
		{name: "one byte key and value", give: []blockEntry{{ikey: "k", value: "v"}}},
		{name: "empty value", give: []blockEntry{{ikey: "tombstone"}}},
		{
			name: "empty value between populated ones",
			give: []blockEntry{{ikey: "a", value: "1"}, {ikey: "b"}, {ikey: "c", value: "3"}},
		},
		{
			name: "keys carrying nul and high bytes",
			give: []blockEntry{{ikey: "k\x00\xff\x00", value: "\x00"}, {ikey: "\xff\xff", value: "\x7f\x80"}},
		},
		{
			name: "lengths needing a two byte uvarint",
			give: []blockEntry{{ikey: strings.Repeat("k", 200), value: strings.Repeat("v", 300)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := buildBlockPayload(tt.give)
			raw := sealBlock(payload)

			if got, want := len(raw), len(payload)+_blockTrailerSize; got != want {
				t.Fatalf("sealed block is %d bytes, want %d", got, want)
			}

			verified, err := verifyBlock(raw)
			if err != nil {
				t.Fatalf("verifyBlock(%x) = %v, want no error", raw, err)
			}
			if !bytes.Equal(verified, payload) {
				t.Fatalf("verified payload = %x, want %x", verified, payload)
			}

			got, err := collectBlockEntries(verified)
			if err != nil {
				t.Fatalf("iterate payload: %v", err)
			}
			if !slices.Equal(got, tt.give) {
				t.Errorf("entries = %q, want %q", got, tt.give)
			}
		})
	}
}

func TestSealBlock_MatchesGoldenBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []blockEntry
		want string
	}{
		{name: "empty block", give: nil, want: "0051537d52"},
		{
			name: "two entries, the second with an empty value",
			give: []blockEntry{{ikey: "ab", value: "X"}, {ikey: "c"}},
			want: "02016162580100630084adf174",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := buildBlockPayload(tt.give)

			if got := hex.EncodeToString(sealBlock(payload)); got != tt.want {
				t.Fatalf("sealBlock(%x) = %s, want %s", payload, got, tt.want)
			}

			golden, err := hex.DecodeString(tt.want)
			if err != nil {
				t.Fatalf("decode golden: %v", err)
			}
			verified, err := verifyBlock(golden)
			if err != nil {
				t.Fatalf("verifyBlock(golden) = %v, want no error", err)
			}
			if !bytes.Equal(verified, payload) {
				t.Errorf("golden payload = %x, want %x", verified, payload)
			}
		})
	}
}

func TestVerifyBlock_RejectsCorruptBlocks(t *testing.T) {
	t.Parallel()

	raw := sealBlock(buildBlockPayload([]blockEntry{{ikey: "ab", value: "X"}, {ikey: "c"}}))

	tests := []struct {
		name string
		give []byte
	}{
		{name: "flipped payload byte", give: flipBlockByte(raw, 0)},
		{name: "flipped last payload byte", give: flipBlockByte(raw, len(raw)-_blockTrailerSize-1)},
		{name: "flipped type byte", give: flipBlockByte(raw, len(raw)-_blockTrailerSize)},
		{name: "flipped crc byte", give: flipBlockByte(raw, len(raw)-1)},
		{name: "truncated below the trailer size", give: raw[:_blockTrailerSize-1]},
		{name: "empty input", give: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := verifyBlock(tt.give)
			if !errors.Is(err, errBlockCorrupt) {
				t.Fatalf("verifyBlock(%x) error = %v, want %v", tt.give, err, errBlockCorrupt)
			}
			if payload != nil {
				t.Errorf("payload = %x, want nil", payload)
			}
		})
	}
}

func TestBlockIter_RejectsCorruptPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []byte
	}{
		{name: "truncated key length varint", give: []byte{0x80}},
		{name: "missing value length varint", give: []byte{0x01}},
		{name: "key length overruns the payload", give: []byte{0x05, 0x00, 'a', 'b'}},
		{name: "value length overruns the payload", give: []byte{0x01, 0x04, 'a', 'b'}},
		{name: "trailing garbage after a complete entry", give: []byte{0x01, 0x01, 'a', 'b', 0x05}},
		{
			name: "key length above the 32-bit limit",
			give: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := newBlockIter(tt.give)
			for {
				if _, _, ok := it.next(); !ok {
					break
				}
			}

			if err := it.err(); !errors.Is(err, errBlockCorrupt) {
				t.Fatalf("err() = %v, want %v", err, errBlockCorrupt)
			}
			if _, _, ok := it.next(); ok {
				t.Error("next() = true after a corrupt payload, want false")
			}
		})
	}
}

func TestBlockIter_StopsCleanlyOnAnExactlyConsumedPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []blockEntry
	}{
		{name: "no entries", give: nil},
		{name: "one entry", give: []blockEntry{{ikey: "a", value: "1"}}},
		{name: "three entries", give: []blockEntry{{ikey: "a", value: "1"}, {ikey: "b"}, {ikey: "c", value: "3"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			it := newBlockIter(buildBlockPayload(tt.give))
			for range tt.give {
				if _, _, ok := it.next(); !ok {
					t.Fatalf("next() = false before the payload was consumed: %v", it.err())
				}
			}

			if _, _, ok := it.next(); ok {
				t.Error("next() = true past the last entry, want false")
			}
			if err := it.err(); err != nil {
				t.Errorf("err() = %v, want nil", err)
			}
		})
	}
}

func TestBlockBuilder_LastKeyReportsTheLastAddedKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []blockEntry
		want string
	}{
		{name: "single add", give: []blockEntry{{ikey: "a", value: "1"}}, want: "a"},
		{
			name: "several adds",
			give: []blockEntry{{ikey: "aa", value: "1"}, {ikey: "bb"}, {ikey: "cc", value: "3"}},
			want: "cc",
		},
		{
			name: "shorter key after a longer one",
			give: []blockEntry{{ikey: "aaaaaaaa", value: "1"}, {ikey: "b", value: "2"}},
			want: "b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBlockBuilder()
			for _, e := range tt.give {
				b.add([]byte(e.ikey), []byte(e.value))
			}

			if got := string(b.lastKey()); got != tt.want {
				t.Errorf("lastKey() = %q, want %q", got, tt.want)
			}
			b.finish()
			if got := string(b.lastKey()); got != tt.want {
				t.Errorf("lastKey() after finish = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlockBuilder_SizeMatchesTheFinishedPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []blockEntry
		want int
	}{
		{name: "no entries", give: nil, want: 0},
		{name: "empty value", give: []blockEntry{{ikey: "k"}}, want: 3},
		{name: "two entries", give: []blockEntry{{ikey: "ab", value: "X"}, {ikey: "c"}}, want: 8},
		{
			name: "key needing a two byte uvarint",
			give: []blockEntry{{ikey: strings.Repeat("k", 200), value: "v"}},
			want: 204,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBlockBuilder()
			for _, e := range tt.give {
				b.add([]byte(e.ikey), []byte(e.value))

				if got, want := b.size(), len(b.finish()); got != want {
					t.Fatalf("size() = %d, want len(finish()) = %d", got, want)
				}
			}

			if got := b.size(); got != tt.want {
				t.Errorf("size() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBlockBuilder_ResetDropsThePreviousBlock(t *testing.T) {
	t.Parallel()

	first := []blockEntry{{ikey: "aaa", value: "111"}, {ikey: "bbb", value: "222"}}
	second := []blockEntry{{ikey: "z", value: "9"}}

	b := newBlockBuilder()
	if !b.empty() {
		t.Fatal("a new builder reports non-empty")
	}
	for _, e := range first {
		b.add([]byte(e.ikey), []byte(e.value))
	}
	if b.empty() {
		t.Fatal("a builder holding entries reports empty")
	}
	b.finish()

	b.reset()
	if !b.empty() || b.size() != 0 || len(b.finish()) != 0 {
		t.Fatalf("after reset: empty=%t size=%d finish=%d bytes, want true, 0 and 0",
			b.empty(), b.size(), len(b.finish()))
	}
	if got := b.lastKey(); len(got) != 0 {
		t.Errorf("lastKey() after reset = %q, want empty", got)
	}

	for _, e := range second {
		b.add([]byte(e.ikey), []byte(e.value))
	}
	if got, want := b.finish(), buildBlockPayload(second); !bytes.Equal(got, want) {
		t.Fatalf("reused builder payload = %x, want %x", got, want)
	}
	if got := string(b.lastKey()); got != "z" {
		t.Errorf("lastKey() after reuse = %q, want %q", got, "z")
	}

	verified, err := verifyBlock(sealBlock(b.finish()))
	if err != nil {
		t.Fatalf("verifyBlock after reuse = %v, want no error", err)
	}
	got, err := collectBlockEntries(verified)
	if err != nil {
		t.Fatalf("iterate reused block: %v", err)
	}
	if !slices.Equal(got, second) {
		t.Errorf("entries = %q, want %q", got, second)
	}
}
