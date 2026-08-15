package batch_test

import (
	"bytes"
	"testing"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/keys"
)

func TestSeal_MatchesFixedVector(t *testing.T) {
	t.Parallel()

	b := batch.New()
	b.Put([]byte("k1"), []byte("v1"))
	b.Delete([]byte("k2"))

	want := goldenPayload(t)
	if got := b.Seal(7); !bytes.Equal(got, want) {
		t.Errorf("sealed payload = %x, want %x", got, want)
	}
}

func TestBatch_RoundTripsThroughReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		giveSeqBase keys.Seq
		give        []entry
	}{
		{
			name:        "empty batch",
			giveSeqBase: 1,
		},
		{
			name:        "single set",
			giveSeqBase: 42,
			give: []entry{
				{kind: keys.KindSet, key: []byte("alpha"), value: []byte("one")},
			},
		},
		{
			name:        "sets and deletes interleaved",
			giveSeqBase: 100,
			give: []entry{
				{kind: keys.KindSet, key: []byte("a"), value: []byte("1")},
				{kind: keys.KindDelete, key: []byte("b")},
				{kind: keys.KindSet, key: []byte("c"), value: bytes.Repeat([]byte{0xab}, 300)},
				{kind: keys.KindDelete, key: bytes.Repeat([]byte("k"), 1000)},
			},
		},
		{
			name:        "set with an empty value",
			giveSeqBase: 7,
			give: []entry{
				{kind: keys.KindSet, key: []byte("empty")},
			},
		},
		{
			name:        "one thousand entries",
			giveSeqBase: 1 << 32,
			give:        manyEntries(1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := batch.New()
			for _, e := range tt.give {
				if e.kind == keys.KindSet {
					b.Put(e.key, e.value)

					continue
				}
				b.Delete(e.key)
			}

			if got := b.Count(); got != uint32(len(tt.give)) {
				t.Errorf("Count = %d, want %d", got, len(tt.give))
			}
			payload := b.Seal(tt.giveSeqBase)
			if got := b.Len(); got != len(payload) {
				t.Errorf("Len = %d, want %d", got, len(payload))
			}

			assertDecodes(t, payload, tt.giveSeqBase, tt.give)
		})
	}
}

func TestBatch_ResetDropsPreviousEntries(t *testing.T) {
	t.Parallel()

	b := batch.New()
	b.Put([]byte("first"), []byte("one"))
	first := bytes.Clone(b.Seal(3))

	b.Reset()
	if got := b.Count(); got != 0 {
		t.Errorf("Count after Reset = %d, want 0", got)
	}
	if got, want := b.Len(), batch.New().Len(); got != want {
		t.Errorf("Len after Reset = %d, want %d", got, want)
	}

	b.Delete([]byte("second"))
	second := b.Seal(11)
	if bytes.Contains(second, []byte("first")) {
		t.Errorf("payload after Reset = %x, want no trace of the previous batch", second)
	}

	assertDecodes(t, first, 3, []entry{{kind: keys.KindSet, key: []byte("first"), value: []byte("one")}})
	assertDecodes(t, second, 11, []entry{{kind: keys.KindDelete, key: []byte("second")}})
}
