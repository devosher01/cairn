package manifest_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

func TestDecode_RejectsCorruptInput(t *testing.T) {
	t.Parallel()

	golden := goldenBytes(t)
	payload := body(golden)
	goldenKeyOff := lastKeyPrefixOff(t, golden, keys.TrailerSize+1)

	var tail manifest.State
	tail.Levels[6] = []manifest.Table{
		{
			FileNum:    1,
			Size:       2,
			EntryCount: 3,
			Smallest:   ikey("a", 1, keys.KindSet),
			Largest:    ikey(longKey(100), 2, keys.KindSet),
		},
		{
			FileNum:    0x1111,
			Size:       0x2222,
			EntryCount: 0x3333,
			Smallest:   ikey("b", 1, keys.KindSet),
			Largest:    ikey(longKey(100), 2, keys.KindSet),
		},
	}
	tailRaw := manifest.Encode(tail)
	tailPayload := body(tailRaw)
	tailKeyOff := lastKeyPrefixOff(t, tailRaw, 100+keys.TrailerSize)
	tailTableOff := tableOff(t, tailPayload, 0x1111, 0x2222, 0x3333)

	var head manifest.State
	head.Levels[0] = []manifest.Table{
		{
			FileNum:    1,
			Size:       2,
			EntryCount: 3,
			Smallest:   ikey("a", 1, keys.KindSet),
			Largest:    ikey(longKey(60), 2, keys.KindSet),
		},
	}
	headPayload := body(manifest.Encode(head))

	var shortKey manifest.State
	shortKey.Levels[0] = []manifest.Table{
		{FileNum: 1, Size: 2, EntryCount: 3, Smallest: []byte("ab"), Largest: []byte("cd")},
	}

	var unordered manifest.State
	unordered.Levels[0] = []manifest.Table{
		{
			FileNum:    1,
			Size:       2,
			EntryCount: 3,
			Smallest:   ikey("z", 1, keys.KindSet),
			Largest:    ikey("a", 1, keys.KindSet),
		},
	}

	tests := []struct {
		name string
		give []byte
	}{
		{name: "file shorter than the fixed prefix", give: slices.Clone(golden[:32])},
		{name: "bad magic", give: seal(patch(payload, _magicOff, 'X'))},
		{name: "bad version", give: seal(patch(payload, _versionOff, 2))},
		{name: "level count six", give: seal(patch(payload, _levelCountOff, 6))},
		{name: "level count eight", give: seal(patch(payload, _levelCountOff, 8))},
		{name: "bad crc", give: patch(golden, len(golden)-1, golden[len(golden)-1]^0xff)},
		{name: "table count overruns the file", give: seal(patch(payload, _level0CountOff, 0xff, 0xff, 0xff, 0xff))},
		{name: "table count truncated", give: seal(headPayload[:len(headPayload)-6*_u32Len])},
		{name: "table file number truncated", give: seal(tailPayload[:tailTableOff+4])},
		{name: "table size truncated", give: seal(tailPayload[:tailTableOff+12])},
		{name: "table entry count truncated", give: seal(tailPayload[:tailTableOff+20])},
		{name: "smallest key length prefix missing", give: seal(tailPayload[:tailTableOff+24])},
		{name: "largest key length prefix truncated", give: seal(append(tailPayload[:tailKeyOff:tailKeyOff], 0x80))},
		{name: "key length above the maximum", give: seal(patch(tailPayload, tailKeyOff, 0xff, 0xff, 0xff, 0xff, 0x1f))},
		{name: "key length overruns the file", give: seal(patch(payload, goldenKeyOff, 0x7f))},
		{name: "key shorter than the trailer", give: manifest.Encode(shortKey)},
		{name: "smallest key above largest", give: manifest.Encode(unordered)},
		{name: "trailing bytes before the crc", give: seal(append(payload, 0xaa))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := manifest.Decode(tt.give)
			if !errors.Is(err, manifest.ErrCorrupt) {
				t.Fatalf("Decode returned error %v, want one wrapping ErrCorrupt", err)
			}
			if !reflect.DeepEqual(got, manifest.State{}) {
				t.Errorf("Decode returned state %+v, want the zero state", got)
			}
		})
	}
}

func TestDecode_RejectsEveryPrefixOfTheGoldenFile(t *testing.T) {
	t.Parallel()

	golden := goldenBytes(t)
	for n := range len(golden) {
		if _, err := manifest.Decode(golden[:n]); !errors.Is(err, manifest.ErrCorrupt) {
			t.Fatalf("Decode of the first %d bytes returned error %v, want one wrapping ErrCorrupt", n, err)
		}
	}
}

func TestDecode_RejectsEverySingleByteFlipInTheGoldenFile(t *testing.T) {
	t.Parallel()

	golden := goldenBytes(t)
	for i := range golden {
		give := patch(golden, i, golden[i]^0xff)
		if _, err := manifest.Decode(give); !errors.Is(err, manifest.ErrCorrupt) {
			t.Fatalf("Decode with byte %d flipped returned error %v, want one wrapping ErrCorrupt", i, err)
		}
	}
}
