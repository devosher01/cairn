package manifest_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

func FuzzDecode(f *testing.F) {
	golden := goldenBytes(f)
	payload := body(golden)

	f.Add(golden)
	f.Add(manifest.Encode(manifest.State{}))
	f.Add(manifest.Encode(campaignStateB()))
	f.Add(slices.Clone(golden[:32]))
	f.Add(seal(patch(payload, _magicOff, 'X')))
	f.Add(seal(patch(payload, _versionOff, 2)))
	f.Add(seal(patch(payload, _levelCountOff, 6)))
	f.Add(seal(patch(payload, _levelCountOff, 8)))
	f.Add(seal(patch(payload, _level0CountOff, 0xff, 0xff, 0xff, 0xff)))
	f.Add(seal(payload[:len(payload)-8]))
	f.Add(seal(append(payload, 0xaa)))
	f.Add(patch(golden, len(golden)-1, golden[len(golden)-1]^0xff))

	f.Fuzz(func(t *testing.T, give []byte) {
		checkDecode(t, give)
		if len(give) >= _crcLen {
			checkDecode(t, seal(body(give)))
		}
	})
}

func checkDecode(t *testing.T, give []byte) {
	t.Helper()

	got, err := manifest.Decode(give)
	if err != nil {
		if !errors.Is(err, manifest.ErrCorrupt) {
			t.Fatalf("Decode error = %v, want one wrapping ErrCorrupt", err)
		}

		return
	}

	for _, level := range got.Levels {
		for _, table := range level {
			if len(table.Smallest) < keys.TrailerSize+1 || len(table.Largest) < keys.TrailerSize+1 {
				t.Fatalf("Decode accepted table %+v with a key shorter than an internal key", table)
			}
			if keys.Compare(table.Smallest, table.Largest) > 0 {
				t.Fatalf("Decode accepted table %+v with its keys out of order", table)
			}
		}
	}

	again, err := manifest.Decode(manifest.Encode(got))
	if err != nil {
		t.Fatalf("re-encoded state does not decode: %v", err)
	}
	if !reflect.DeepEqual(again, got) {
		t.Errorf("re-encoded state decodes to %+v, want %+v", again, got)
	}
	if !reflect.DeepEqual(got.Clone(), got) {
		t.Errorf("Clone returned %+v, want %+v", got.Clone(), got)
	}
}
