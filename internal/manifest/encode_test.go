package manifest_test

import (
	"bytes"
	"flag"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

var _updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata")

func TestEncodeDecode_RoundTripsState(t *testing.T) {
	t.Parallel()

	var empty manifest.State

	var single manifest.State
	single.NextFileNum = 1
	single.Levels[0] = []manifest.Table{
		{
			FileNum:    1,
			Size:       64,
			EntryCount: 1,
			Smallest:   ikey("k", 1, keys.KindSet),
			Largest:    ikey("k", 1, keys.KindSet),
		},
	}

	var extremes manifest.State
	extremes.NextFileNum = math.MaxUint64
	extremes.LastSeq = keys.MaxSeq
	extremes.OldestWAL = math.MaxUint64
	extremes.Levels[6] = []manifest.Table{
		{
			FileNum:    math.MaxUint64,
			Size:       math.MaxUint64,
			EntryCount: math.MaxUint64,
			Smallest:   ikey(longKey(1), keys.MaxSeq, keys.KindDelete),
			Largest:    ikey(longKey(4096), keys.MaxSeq, keys.KindSet),
		},
	}

	tests := []struct {
		name string
		give manifest.State
	}{
		{name: "empty state", give: empty},
		{name: "one table on the first level", give: single},
		{name: "tables on several levels with gaps between them", give: goldenState()},
		{name: "maximum numbers and a long key", give: extremes},
		{name: "two installs worth of tables", give: campaignStateB()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := manifest.Encode(tt.give)
			if again := manifest.Encode(tt.give); !bytes.Equal(raw, again) {
				t.Errorf("Encode is not deterministic: %x then %x", raw, again)
			}

			got, err := manifest.Decode(raw)
			if err != nil {
				t.Fatalf("Decode returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.give) {
				t.Errorf("Decode(Encode(s)) = %+v, want %+v", got, tt.give)
			}
		})
	}
}

func TestDecode_CopiesKeysOutOfTheInput(t *testing.T) {
	t.Parallel()

	raw := manifest.Encode(goldenState())
	got, err := manifest.Decode(raw)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	want := bytes.Clone(got.Levels[0][0].Smallest)
	for i := range raw {
		raw[i] = 0xff
	}
	if !bytes.Equal(got.Levels[0][0].Smallest, want) {
		t.Errorf("smallest key = %x after overwriting the input, want %x", got.Levels[0][0].Smallest, want)
	}
}

func TestEncode_MatchesGoldenFile(t *testing.T) {
	t.Parallel()

	got := manifest.Encode(goldenState())

	if *_updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath(), got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want := goldenBytes(t)
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode = %x, want %x", got, want)
	}

	state, err := manifest.Decode(want)
	if err != nil {
		t.Fatalf("Decode of the golden file returned error: %v", err)
	}
	if !reflect.DeepEqual(state, goldenState()) {
		t.Errorf("the golden file decodes to %+v, want %+v", state, goldenState())
	}
}

func TestGolden_HeaderLayout(t *testing.T) {
	t.Parallel()

	raw := goldenBytes(t)
	if got := string(raw[_magicOff : _magicOff+8]); got != "CAIRNMAN" {
		t.Errorf("magic = %q, want %q", got, "CAIRNMAN")
	}
	if got := raw[_versionOff]; got != 1 {
		t.Errorf("version byte = %d, want 1", got)
	}
	if got := raw[_levelCountOff]; got != manifest.NumLevels {
		t.Errorf("level count byte = %d, want %d", got, manifest.NumLevels)
	}
}
