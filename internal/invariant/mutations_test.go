package invariant_test

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

const (
	_overlapLeftNum     = 10
	_overlapRightNum    = 11
	_overlapNextFileNum = 12
	_orphanNum          = 6
	_staleWALNum        = 6
	_lowNextFileNum     = 3
	_pairEntrySize      = 18
	_manifestTmpName    = "MANIFEST.tmp"
)

type mutation func(t *testing.T, fs env.FS, s manifest.State)

func truncateReferencedTable(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	name := sstFileName(s.Levels[1][0].FileNum)
	raw := readFile(t, fs, name)
	writeFile(t, fs, name, raw[:len(raw)-1])
}

func removeReferencedTable(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	if err := fs.Remove(sstFileName(s.Levels[1][0].FileNum)); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
}

func removeManifest(t *testing.T, fs env.FS, _ manifest.State) {
	t.Helper()

	if err := fs.Remove(manifest.FileName); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
}

func corruptManifest(t *testing.T, fs env.FS, _ manifest.State) {
	t.Helper()

	raw := readFile(t, fs, manifest.FileName)
	raw[len(raw)/2] ^= 0xFF
	writeFile(t, fs, manifest.FileName, raw)
}

func overlapLevelOne(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	left := writeTable(t, fs, _overlapLeftNum, []string{"m", "o"}, 10)
	right := writeTable(t, fs, _overlapRightNum, []string{"n", "p"}, 12)

	doctored := s.Clone()
	doctored.Levels[1] = []manifest.Table{left, right}
	doctored.NextFileNum = _overlapNextFileNum
	install(t, fs, doctored)
}

func unsortLevelOne(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	doctored := s.Clone()
	doctored.Levels[1] = []manifest.Table{doctored.Levels[1][1], doctored.Levels[1][0]}
	install(t, fs, doctored)
}

func corruptDataBlock(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	name := sstFileName(s.Levels[1][0].FileNum)
	raw := readFile(t, fs, name)
	raw[0] ^= 0xFF
	writeFile(t, fs, name, raw)
}

func swapFirstTwoEntries(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	name := sstFileName(s.Levels[1][0].FileNum)
	raw := readFile(t, fs, name)

	footer := raw[len(raw)-_footerSize:]
	filterOffset := binary.LittleEndian.Uint64(footer[_filterOffsetAt:])

	block := raw[:filterOffset]
	payload := block[:len(block)-_blockTrailerSize]
	if len(payload) != 2*_pairEntrySize {
		t.Fatalf("the data block holds %d bytes, want two entries of %d", len(payload), _pairEntrySize)
	}

	swapped := make([]byte, 0, len(payload))
	swapped = append(swapped, payload[_pairEntrySize:]...)
	swapped = append(swapped, payload[:_pairEntrySize]...)
	copy(payload, swapped)
	binary.LittleEndian.PutUint32(block[len(block)-_crcSize:], crc32.Checksum(block[:len(block)-_crcSize], _castagnoli))
	writeFile(t, fs, name, raw)
}

func overstateEntryCount(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	doctored := s.Clone()
	doctored.Levels[2][0].EntryCount++
	install(t, fs, doctored)
}

func lowerSmallestKey(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	doctored := s.Clone()
	doctored.Levels[2][0].Smallest = keys.Append(nil, []byte("aa"), 1, keys.KindSet)
	install(t, fs, doctored)
}

func emptyFilterBlock(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	name := sstFileName(s.Levels[1][0].FileNum)
	raw := readFile(t, fs, name)

	footer := raw[len(raw)-_footerSize:]
	offset := binary.LittleEndian.Uint64(footer[_filterOffsetAt:])
	length := binary.LittleEndian.Uint64(footer[_filterLengthAt:])

	block := raw[offset : offset+length]
	clear(block[_filterHeaderSize : len(block)-_blockTrailerSize])
	binary.LittleEndian.PutUint32(block[len(block)-_crcSize:], crc32.Checksum(block[:len(block)-_crcSize], _castagnoli))
	writeFile(t, fs, name, raw)
}

func lowerLastSeq(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	doctored := s.Clone()
	doctored.LastSeq = 0
	install(t, fs, doctored)
}

func lowerNextFileNum(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	doctored := s.Clone()
	doctored.NextFileNum = _lowNextFileNum
	install(t, fs, doctored)
}

func addOrphanTable(t *testing.T, fs env.FS, _ manifest.State) {
	t.Helper()

	writeTable(t, fs, _orphanNum, []string{"x", "y"}, 1)
}

func addStaleWAL(t *testing.T, fs env.FS, _ manifest.State) {
	t.Helper()

	writeFile(t, fs, walFileName(_staleWALNum), []byte("CAIRNWAL"))
}

func addManifestTmp(t *testing.T, fs env.FS, _ manifest.State) {
	t.Helper()

	writeFile(t, fs, _manifestTmpName, []byte("half a manifest"))
}
