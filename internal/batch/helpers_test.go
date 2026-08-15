package batch_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/keys"
)

const _goldenHex = "07000000000000000200000001026b3102763102026b32"

type entry struct {
	kind  keys.Kind
	key   []byte
	value []byte
}

type decoded struct {
	seqBase keys.Seq
	count   uint32
	entries []batch.Entry
}

func decodeAll(payload []byte) (decoded, bool) {
	r, err := batch.NewReader(payload)
	if err != nil {
		return decoded{}, false
	}

	out := decoded{seqBase: r.SeqBase(), count: r.Count()}
	for {
		e, ok := r.Next()
		if !ok {
			break
		}
		out.entries = append(out.entries, e)
	}
	if r.Err() != nil {
		return decoded{}, false
	}

	return out, true
}

func goldenPayload(tb testing.TB) []byte {
	tb.Helper()

	data, err := hex.DecodeString(_goldenHex)
	if err != nil {
		tb.Fatalf("decode golden vector: %v", err)
	}

	return data
}

func build(seqBase keys.Seq, entries []entry) []byte {
	b := batch.New()
	for _, e := range entries {
		if e.kind == keys.KindSet {
			b.Put(e.key, e.value)

			continue
		}
		b.Delete(e.key)
	}

	return b.Seal(seqBase)
}

func manyEntries(n int) []entry {
	out := make([]entry, 0, n)
	for i := range n {
		key := fmt.Appendf(nil, "key-%06d", i)
		if i%3 == 0 {
			out = append(out, entry{kind: keys.KindDelete, key: key})

			continue
		}
		out = append(out, entry{kind: keys.KindSet, key: key, value: fmt.Appendf(nil, "value-%d", i)})
	}

	return out
}

func header(seqBase uint64, count uint32, tail ...byte) []byte {
	out := binary.LittleEndian.AppendUint64(nil, seqBase)
	out = binary.LittleEndian.AppendUint32(out, count)

	return append(out, tail...)
}

func assertDecodes(t *testing.T, payload []byte, wantSeqBase keys.Seq, want []entry) {
	t.Helper()

	r, err := batch.NewReader(payload)
	if err != nil {
		t.Fatalf("NewReader returned error: %v", err)
	}
	if got := r.SeqBase(); got != wantSeqBase {
		t.Errorf("SeqBase = %d, want %d", got, wantSeqBase)
	}
	if got := r.Count(); got != uint32(len(want)) {
		t.Errorf("Count = %d, want %d", got, len(want))
	}

	decoded := 0
	for {
		got, ok := r.Next()
		if !ok {
			break
		}
		if decoded >= len(want) {
			t.Fatalf("decoded entry %d beyond the expected %d", decoded, len(want))
		}
		assertEntry(t, got, want[decoded], wantSeqBase+keys.Seq(decoded))
		decoded++
	}

	if err := r.Err(); err != nil {
		t.Fatalf("Err after the walk = %v, want nil", err)
	}
	if decoded != len(want) {
		t.Errorf("decoded %d entries, want %d", decoded, len(want))
	}
}

func assertEntry(t *testing.T, got batch.Entry, want entry, wantSeq keys.Seq) {
	t.Helper()

	if got.Seq != wantSeq {
		t.Errorf("entry seq = %d, want %d", got.Seq, wantSeq)
	}
	if got.Kind != want.kind {
		t.Errorf("entry kind = %d, want %d", got.Kind, want.kind)
	}
	if !bytes.Equal(got.Key, want.key) {
		t.Errorf("entry key = %q, want %q", got.Key, want.key)
	}
	if !bytes.Equal(got.Value, want.value) {
		t.Errorf("entry value = %q, want %q", got.Value, want.value)
	}
}
