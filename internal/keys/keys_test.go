package keys_test

import (
	"bytes"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
)

func TestAppend_MatchesFixedVector(t *testing.T) {
	t.Parallel()

	got := keys.Append(nil, []byte("key"), 0x0102, keys.KindSet)
	want := []byte{'k', 'e', 'y', 0x01, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}

	if !bytes.Equal(got, want) {
		t.Errorf("encoded key = %x, want %x", got, want)
	}
}

func TestTrailer_RoundTripsSeqAndKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		giveSeq  keys.Seq
		giveKind keys.Kind
	}{
		{name: "set at seq one", giveSeq: 1, giveKind: keys.KindSet},
		{name: "delete at a large seq", giveSeq: 1 << 40, giveKind: keys.KindDelete},
		{name: "set at the maximum seq", giveSeq: keys.MaxSeq, giveKind: keys.KindSet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ikey := keys.Append(nil, []byte("user"), tt.giveSeq, tt.giveKind)

			if got := keys.UserKey(ikey); !bytes.Equal(got, []byte("user")) {
				t.Errorf("user key = %q, want %q", got, "user")
			}
			seq, kind := keys.Trailer(ikey)
			if seq != tt.giveSeq || kind != tt.giveKind {
				t.Errorf("trailer = (%d, %d), want (%d, %d)", seq, kind, tt.giveSeq, tt.giveKind)
			}
		})
	}
}

func TestCompare_OrdersUserAscendingThenSeqDescending(t *testing.T) {
	t.Parallel()

	ordered := [][]byte{
		keys.AppendSeek(nil, []byte("a"), 9),
		keys.Append(nil, []byte("a"), 9, keys.KindDelete),
		keys.Append(nil, []byte("a"), 5, keys.KindSet),
		keys.Append(nil, []byte("a"), 1, keys.KindSet),
		keys.AppendSeek(nil, []byte("b"), keys.MaxSeq),
		keys.Append(nil, []byte("b"), keys.MaxSeq, keys.KindSet),
		keys.Append(nil, []byte("b"), 2, keys.KindSet),
		keys.Append(nil, []byte("ba"), 100, keys.KindSet),
	}

	for i, a := range ordered {
		if got := keys.Compare(a, a); got != 0 {
			t.Errorf("Compare(%x, itself) = %d, want 0", a, got)
		}
		for _, b := range ordered[i+1:] {
			if got := keys.Compare(a, b); got >= 0 {
				t.Errorf("Compare(%x, %x) = %d, want negative", a, b, got)
			}
			if got := keys.Compare(b, a); got <= 0 {
				t.Errorf("Compare(%x, %x) = %d, want positive", b, a, got)
			}
		}
	}
}

func TestKind_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give keys.Kind
		want bool
	}{
		{name: "set is valid", give: keys.KindSet, want: true},
		{name: "delete is valid", give: keys.KindDelete, want: true},
		{name: "zero is invalid", give: 0, want: false},
		{name: "seek marker is invalid", give: 0xFF, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.give.Valid(); got != tt.want {
				t.Errorf("Valid(%d) = %t, want %t", tt.give, got, tt.want)
			}
		})
	}
}
