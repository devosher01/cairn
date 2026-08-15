package batch_test

import (
	"bytes"
	"testing"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/keys"
)

func FuzzReader(f *testing.F) {
	golden := goldenPayload(f)
	f.Add(golden)
	f.Add(golden[:9])
	f.Add(append(bytes.Clone(golden), 0xff))

	f.Fuzz(func(t *testing.T, give []byte) {
		got, ok := decodeAll(give)
		if !ok {
			return
		}
		if uint32(len(got.entries)) != got.count {
			t.Fatalf("decoded %d entries, want the declared %d", len(got.entries), got.count)
		}

		b := batch.New()
		for _, e := range got.entries {
			if e.Kind == keys.KindSet {
				b.Put(e.Key, e.Value)

				continue
			}
			b.Delete(e.Key)
		}

		payload := b.Seal(got.seqBase)
		again, ok := decodeAll(payload)
		if !ok {
			t.Fatalf("re-encoded payload %x does not decode cleanly", payload)
		}
		if again.seqBase != got.seqBase {
			t.Errorf("re-encoded seq base = %d, want %d", again.seqBase, got.seqBase)
		}
		if len(again.entries) != len(got.entries) {
			t.Fatalf("re-encoded %d entries, want %d", len(again.entries), len(got.entries))
		}

		for i, want := range got.entries {
			e := again.entries[i]
			if e.Seq != want.Seq || e.Kind != want.Kind {
				t.Errorf("re-encoded entry %d = (seq %d, kind %d), want (seq %d, kind %d)",
					i, e.Seq, e.Kind, want.Seq, want.Kind)
			}
			if !bytes.Equal(e.Key, want.Key) || !bytes.Equal(e.Value, want.Value) {
				t.Errorf("re-encoded entry %d = (%q, %q), want (%q, %q)", i, e.Key, e.Value, want.Key, want.Value)
			}
		}
	})
}
