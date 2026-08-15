package invariant_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/invariant"
)

const (
	_freshSeed   uint64 = 9101
	_flushSeed   uint64 = 9102
	_levelSeed   uint64 = 9103
	_crashSeed   uint64 = 9104
	_corruptSeed uint64 = 9105
)

const (
	_smallMemtable  = 256
	_smallBlock     = 64
	_smallTarget    = 512
	_smallBaseLevel = 512
	_flushedKeys    = 200
	_leveledKeys    = 400
	_minDeepTables  = 2
)

func TestCheck_AcceptsAFreshDatabase(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_freshSeed)
	db := openEngine(t, sim, cairn.Options{DisableAutoCompaction: true})
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	requireClean(t, sim.Env().FS)
}

func TestCheck_AcceptsAFlushedDatabase(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_flushSeed)
	db := openEngine(t, sim, cairn.Options{
		DisableAutoCompaction: true,
		MemtableSize:          _smallMemtable,
		BlockSize:             _smallBlock,
	})
	putRange(t, db, _flushedKeys)

	if got := db.Metrics().Levels[0].Tables; got < _minDeepTables {
		t.Fatalf("level 0 holds %d tables, want at least %d", got, _minDeepTables)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	requireClean(t, sim.Env().FS)
}

func TestCheck_AcceptsACompactedDatabase(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_levelSeed)
	db := openEngine(t, sim, cairn.Options{
		MemtableSize:     _smallMemtable,
		BlockSize:        _smallBlock,
		TargetFileSize:   _smallTarget,
		BaseLevelSize:    _smallBaseLevel,
		L0CompactTrigger: 1,
		L0StallTrigger:   2,
	})
	putRange(t, db, _leveledKeys)

	levels := db.Metrics().Levels
	deep := 0
	for _, level := range levels[1:] {
		deep += level.Tables
	}
	if deep < _minDeepTables {
		t.Fatalf("levels 1 to 6 hold %d tables, want at least %d", deep, _minDeepTables)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	requireClean(t, sim.Env().FS)
}

func TestCheck_RejectsACorruptEngineTable(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_corruptSeed)
	db := openEngine(t, sim, cairn.Options{
		DisableAutoCompaction: true,
		MemtableSize:          _smallMemtable,
		BlockSize:             _smallBlock,
	})
	putRange(t, db, _flushedKeys)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	fs := sim.Env().FS
	name := firstTableName(t, fs)
	raw := readFile(t, fs, name)
	raw[0] ^= 0xFF
	writeFile(t, fs, name, raw)

	requireViolation(t, invariant.Check(fs), "I4")
	requireViolation(t, invariant.CheckCrashDisk(fs), "I4")
}

func TestCheckCrashDisk_AcceptsADiskCrashedMidFlush(t *testing.T) {
	t.Parallel()

	sim := simenv.New(_crashSeed)
	db := openEngine(t, sim, cairn.Options{
		DisableAutoCompaction: true,
		MemtableSize:          _smallMemtable,
		BlockSize:             _smallBlock,
	})
	putRange(t, db, _flushedKeys)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	op, ok := lastTableCreate(sim.Ops())
	if !ok {
		t.Fatal("the op log holds no sstable create")
	}
	disk := sim.MaterializeCrash(simenv.CrashPoint{Op: op + 1, Mode: simenv.CrashPrefix})

	if err := invariant.CheckCrashDisk(disk); err != nil {
		t.Fatalf("CheckCrashDisk returned error: %v", err)
	}
	if err := invariant.Check(disk); !errors.Is(err, invariant.ErrViolated) {
		t.Fatalf("Check returned %v, want a violation of the unreferenced output file", err)
	}
}

func firstTableName(t *testing.T, fs env.FS) string {
	t.Helper()

	names, err := fs.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".sst") {
			return name
		}
	}
	t.Fatal("the directory holds no sstable")

	return ""
}

func lastTableCreate(ops []simenv.Op) (int, bool) {
	for i, op := range slices.Backward(ops) {
		if op.Kind == simenv.OpCreate && strings.HasSuffix(op.Name, ".sst") {
			return i, true
		}
	}

	return 0, false
}
