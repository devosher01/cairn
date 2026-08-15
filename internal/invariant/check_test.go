package invariant_test

import (
	"testing"

	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/invariant"
	"github.com/devosher01/cairn/internal/manifest"
)

const (
	_validSeed     uint64 = 9001
	_emptySeed     uint64 = 9002
	_largeSeed     uint64 = 9003
	_violationSeed uint64 = 9004
)

const (
	_largeTableKeys   = 600
	_largeNextFileNum = 2
	_largeLastSeq     = 600
)

func TestCheck_AcceptsAHandBuiltDatabase(t *testing.T) {
	t.Parallel()

	fs := simenv.New(_validSeed).Env().FS
	installValid(t, fs)

	requireClean(t, fs)
}

func TestCheck_AcceptsAnEmptyDirectory(t *testing.T) {
	t.Parallel()

	requireClean(t, simenv.New(_emptySeed).Env().FS)
}

func TestCheck_AcceptsATableWiderThanTheKeySample(t *testing.T) {
	t.Parallel()

	fs := simenv.New(_largeSeed).Env().FS
	state := manifest.State{NextFileNum: _largeNextFileNum, LastSeq: _largeLastSeq}
	state.Levels[1] = []manifest.Table{writeTable(t, fs, 1, userKeys(_largeTableKeys), 1)}
	install(t, fs, state)

	requireClean(t, fs)
}

func TestCheck_RejectsViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		give        mutation
		wantTag     string
		wantOnCrash bool
	}{
		{name: "referenced table truncated", give: truncateReferencedTable, wantTag: "I1", wantOnCrash: true},
		{name: "referenced table missing", give: removeReferencedTable, wantTag: "I1", wantOnCrash: true},
		{name: "manifest gone while data files remain", give: removeManifest, wantTag: "I1", wantOnCrash: true},
		{name: "manifest byte flipped", give: corruptManifest, wantTag: "I1", wantOnCrash: true},
		{name: "level one tables overlap", give: overlapLevelOne, wantTag: "I2", wantOnCrash: true},
		{name: "level one tables out of order", give: unsortLevelOne, wantTag: "I2", wantOnCrash: true},
		{name: "entries out of order inside a table", give: swapFirstTwoEntries, wantTag: "I3", wantOnCrash: true},
		{name: "entry count above the stored entries", give: overstateEntryCount, wantTag: "I3", wantOnCrash: true},
		{name: "smallest key below the stored keys", give: lowerSmallestKey, wantTag: "I3", wantOnCrash: true},
		{name: "data block byte flipped", give: corruptDataBlock, wantTag: "I4", wantOnCrash: true},
		{name: "filter block empty of its own keys", give: emptyFilterBlock, wantTag: "I5", wantOnCrash: true},
		{name: "last sequence below the stored keys", give: lowerLastSeq, wantTag: "I6", wantOnCrash: true},
		{name: "next file number below a referenced table", give: lowerNextFileNum, wantTag: "I7", wantOnCrash: true},
		{name: "orphan sstable left behind", give: addOrphanTable, wantTag: "I8"},
		{name: "write-ahead log below the oldest live one", give: addStaleWAL, wantTag: "I8"},
		{name: "manifest tmp file left behind", give: addManifestTmp, wantTag: "I8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := simenv.New(_violationSeed).Env().FS
			state := installValid(t, fs)
			tt.give(t, fs, state)

			requireViolation(t, invariant.Check(fs), tt.wantTag)

			crash := invariant.CheckCrashDisk(fs)
			if !tt.wantOnCrash {
				if crash != nil {
					t.Fatalf("CheckCrashDisk returned error %v, want nil", crash)
				}

				return
			}
			requireViolation(t, crash, tt.wantTag)
		})
	}
}
