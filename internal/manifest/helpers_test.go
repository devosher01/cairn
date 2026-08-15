package manifest_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

var errBoom = errors.New("boom")

const (
	_crcLen         = 4
	_u32Len         = 4
	_magicOff       = 0
	_versionOff     = 8
	_levelCountOff  = 36
	_level0CountOff = 37
)

var _crcTable = crc32.MakeTable(crc32.Castagnoli)

func ikey(user string, seq keys.Seq, kind keys.Kind) []byte {
	return keys.Append(nil, []byte(user), seq, kind)
}

func goldenState() manifest.State {
	var s manifest.State
	s.NextFileNum = 42
	s.LastSeq = 1234
	s.OldestWAL = 7
	s.Levels[0] = []manifest.Table{
		{
			FileNum:    11,
			Size:       4096,
			EntryCount: 12,
			Smallest:   ikey("apple", 3, keys.KindSet),
			Largest:    ikey("banana", 9, keys.KindDelete),
		},
		{
			FileNum:    12,
			Size:       8192,
			EntryCount: 30,
			Smallest:   ikey("cherry", 4, keys.KindSet),
			Largest:    ikey("date", 5, keys.KindSet),
		},
	}
	s.Levels[2] = []manifest.Table{
		{
			FileNum:    9,
			Size:       1 << 20,
			EntryCount: 5000,
			Smallest:   ikey("a", 1, keys.KindSet),
			Largest:    ikey(longKey(200), 2, keys.KindDelete),
		},
	}
	s.Levels[6] = []manifest.Table{
		{
			FileNum:    3,
			Size:       1 << 30,
			EntryCount: 1 << 20,
			Smallest:   ikey("m", 0, keys.KindSet),
			Largest:    ikey("z", 0, keys.KindSet),
		},
	}

	return s
}

func campaignStateA() manifest.State {
	var s manifest.State
	s.NextFileNum = 5
	s.LastSeq = 90
	s.OldestWAL = 2
	s.Levels[0] = []manifest.Table{
		{
			FileNum:    4,
			Size:       1024,
			EntryCount: 8,
			Smallest:   ikey("alpha", 1, keys.KindSet),
			Largest:    ikey("bravo", 2, keys.KindSet),
		},
	}

	return s
}

func campaignStateB() manifest.State {
	var s manifest.State
	s.NextFileNum = 9
	s.LastSeq = 210
	s.OldestWAL = 4
	s.Levels[1] = []manifest.Table{
		{
			FileNum:    6,
			Size:       2048,
			EntryCount: 20,
			Smallest:   ikey("alpha", 1, keys.KindSet),
			Largest:    ikey("delta", 12, keys.KindDelete),
		},
		{
			FileNum:    7,
			Size:       4096,
			EntryCount: 41,
			Smallest:   ikey("echo", 13, keys.KindSet),
			Largest:    ikey(longKey(120), 14, keys.KindSet),
		},
	}

	return s
}

func longKey(n int) string {
	return string(slices.Repeat([]byte("k"), n))
}

func goldenPath() string {
	return filepath.Join("testdata", "manifest_v1.golden")
}

func goldenBytes(t testing.TB) []byte {
	t.Helper()

	raw, err := os.ReadFile(goldenPath())
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	return raw
}

func body(raw []byte) []byte {
	return slices.Clone(raw[:len(raw)-_crcLen])
}

func seal(payload []byte) []byte {
	return binary.LittleEndian.AppendUint32(slices.Clone(payload), crc32.Checksum(payload, _crcTable))
}

func patch(raw []byte, off int, replacement ...byte) []byte {
	out := slices.Clone(raw)
	copy(out[off:], replacement)

	return out
}

func lastKeyPrefixOff(t *testing.T, raw []byte, keyLen int) int {
	t.Helper()

	off := len(raw) - _crcLen - keyLen - 1
	if int(raw[off]) != keyLen {
		t.Fatalf("byte %d = %d, want the trailing key length %d", off, raw[off], keyLen)
	}

	return off
}

func tableOff(t *testing.T, raw []byte, fileNum, size, entries uint64) int {
	t.Helper()

	var fixed []byte
	fixed = binary.LittleEndian.AppendUint64(fixed, fileNum)
	fixed = binary.LittleEndian.AppendUint64(fixed, size)
	fixed = binary.LittleEndian.AppendUint64(fixed, entries)

	off := bytes.Index(raw, fixed)
	if off < 0 {
		t.Fatalf("table %d is not in the encoded manifest", fileNum)
	}

	return off
}

func describePoint(p simenv.CrashPoint) string {
	return fmt.Sprintf("crash at op %d torn %d mode %d seed %d", p.Op, p.Torn, p.Mode, p.ScatterSeed)
}

func installOn(t *testing.T, sim *simenv.Sim, states ...manifest.State) {
	t.Helper()

	for i, s := range states {
		if err := manifest.Install(sim.Env().FS, s); err != nil {
			t.Fatalf("Install %d returned error: %v", i, err)
		}
	}
}

func writeFile(t *testing.T, dir env.FS, name string, data []byte) {
	t.Helper()

	f, err := dir.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync %s: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", name, err)
	}
}
