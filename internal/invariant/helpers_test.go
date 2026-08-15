package invariant_test

import (
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/invariant"
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
	"github.com/devosher01/cairn/internal/sstable"
)

const (
	_dbDir     = "invariant"
	_blockSize = 64
	_bloomBits = 10
)

const (
	_baseNextFileNum = 8
	_baseOldestWAL   = 7
	_baseLastSeq     = 13
)

const (
	_footerSize       = 48
	_filterOffsetAt   = 16
	_filterLengthAt   = 24
	_filterHeaderSize = 1
	_blockTrailerSize = 5
	_crcSize          = 4
)

var _castagnoli = crc32.MakeTable(crc32.Castagnoli)

func installValid(t *testing.T, fs env.FS) manifest.State {
	t.Helper()

	state := manifest.State{
		NextFileNum: _baseNextFileNum,
		LastSeq:     _baseLastSeq,
		OldestWAL:   _baseOldestWAL,
	}
	state.Levels[0] = []manifest.Table{
		writeTable(t, fs, 1, []string{"a", "b", "c"}, 1),
		writeTable(t, fs, 2, []string{"b", "c", "d"}, 4),
	}
	state.Levels[1] = []manifest.Table{
		writeTable(t, fs, 3, []string{"e", "f"}, 7),
		writeTable(t, fs, 4, []string{"g", "h"}, 9),
	}
	state.Levels[2] = []manifest.Table{
		writeTable(t, fs, 5, []string{"i", "j", "k"}, 11),
	}
	install(t, fs, state)

	return state
}

func writeTable(t *testing.T, fs env.FS, num uint64, users []string, baseSeq keys.Seq) manifest.Table {
	t.Helper()

	f, err := fs.Create(sstFileName(num))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	w := sstable.NewWriter(f, sstable.WriterOptions{BlockSize: _blockSize, BloomBitsPerKey: _bloomBits})

	var ikey []byte
	for i, user := range users {
		ikey = keys.Append(ikey[:0], []byte(user), baseSeq+keys.Seq(i), keys.KindSet)
		if err := w.Add(ikey, []byte("value-"+user)); err != nil {
			t.Fatalf("Add returned error: %v", err)
		}
	}
	meta, err := w.Finish()
	if err != nil {
		t.Fatalf("Finish returned error: %v", err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	return manifest.Table{
		FileNum:    num,
		Size:       uint64(meta.Size),
		EntryCount: meta.EntryCount,
		Smallest:   meta.Smallest,
		Largest:    meta.Largest,
	}
}

func install(t *testing.T, fs env.FS, s manifest.State) {
	t.Helper()

	if err := manifest.Install(fs, s); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
}

func readFile(t *testing.T, fs env.FS, name string) []byte {
	t.Helper()

	f, err := fs.Open(name)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size returned error: %v", err)
	}
	raw := make([]byte, size)
	if _, err := f.ReadAt(raw, 0); err != nil {
		t.Fatalf("ReadAt returned error: %v", err)
	}

	return raw
}

func writeFile(t *testing.T, fs env.FS, name string, raw []byte) {
	t.Helper()

	f, err := fs.Create(name)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := f.Write(raw); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func openEngine(t *testing.T, sim *simenv.Sim, opts cairn.Options) *cairn.DB {
	t.Helper()

	opts.Env = sim.Env()
	db, err := cairn.Open(_dbDir, &opts)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	return db
}

func putRange(t *testing.T, db *cairn.DB, count int) {
	t.Helper()

	for i := range count {
		key := fmt.Sprintf("key-%04d", i)
		if err := db.Put([]byte(key), []byte("value-"+key)); err != nil {
			t.Fatalf("Put returned error: %v", err)
		}
	}
}

func userKeys(count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = fmt.Sprintf("key-%04d", i)
	}

	return out
}

func sstFileName(num uint64) string {
	return fmt.Sprintf("%06d.sst", num)
}

func walFileName(num uint64) string {
	return fmt.Sprintf("%06d.wal", num)
}

func requireClean(t *testing.T, fs env.FS) {
	t.Helper()

	if err := invariant.Check(fs); err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if err := invariant.CheckCrashDisk(fs); err != nil {
		t.Fatalf("CheckCrashDisk returned error: %v", err)
	}
}

func requireViolation(t *testing.T, err error, tag string) {
	t.Helper()

	if !errors.Is(err, invariant.ErrViolated) {
		t.Fatalf("error %v does not wrap ErrViolated", err)
	}
	if want := ": " + tag + ": "; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name invariant %s", err, tag)
	}
}
