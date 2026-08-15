package manifest_test

import (
	"reflect"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

func TestState_CloneCopiesEveryLevelAndKey(t *testing.T) {
	t.Parallel()

	original := goldenState()
	clone := original.Clone()
	if !reflect.DeepEqual(clone, original) {
		t.Fatalf("Clone returned %+v, want %+v", clone, original)
	}

	clone.NextFileNum = 99
	clone.LastSeq = 99
	clone.OldestWAL = 99
	clone.Levels[0][0].FileNum = 99
	clone.Levels[0][0].Smallest[0] = 'X'
	clone.Levels[0][0].Largest[0] = 'X'
	clone.Levels[0] = append(clone.Levels[0], manifest.Table{
		FileNum:  99,
		Smallest: ikey("zz", 1, keys.KindSet),
		Largest:  ikey("zz", 1, keys.KindSet),
	})
	clone.Levels[3] = []manifest.Table{{FileNum: 99}}

	if !reflect.DeepEqual(original, goldenState()) {
		t.Errorf("mutating the clone changed the original to %+v", original)
	}
}

func TestState_CloneLeavesTheOriginalFreeToGrow(t *testing.T) {
	t.Parallel()

	original := goldenState()
	clone := original.Clone()

	original.Levels[0] = append(original.Levels[0], manifest.Table{FileNum: 99})
	original.Levels[0][0].Smallest[0] = 'X'

	if !reflect.DeepEqual(clone, goldenState()) {
		t.Errorf("mutating the original changed the clone to %+v", clone)
	}
}
