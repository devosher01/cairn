package simenv_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

func TestMaterializeCrash_NoneKeepsOnlySyncedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		giveSync bool
		want     []byte
	}{
		{name: "unsynced write is lost", giveSync: false, want: nil},
		{name: "synced write survives", giveSync: true, want: []byte("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(1)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", []byte("hello"))
			if tt.giveSync {
				mustSync(t, h)
			}
			mustSyncDir(t, fsys)

			disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

			if got := readFile(t, disk, "a"); !bytes.Equal(got, tt.want) {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaterializeCrash_PrefixKeepsUnsyncedWrites(t *testing.T) {
	t.Parallel()

	sim := simenv.New(2)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", []byte("abcd"))
	mustSync(t, h)
	mustWrite(t, h, []byte("efgh"))

	disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashPrefix})

	if got := readFile(t, disk, "a"); !bytes.Equal(got, []byte("abcdefgh")) {
		t.Errorf("content = %q, want %q", got, "abcdefgh")
	}
}

func TestMaterializeCrash_PrefixCutsTheTornWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		giveTorn int
		want     []byte
	}{
		{name: "torn after one byte", giveTorn: 1, want: []byte("abcde")},
		{name: "torn in the middle", giveTorn: 2, want: []byte("abcdef")},
		{name: "torn before the last byte", giveTorn: 3, want: []byte("abcdefg")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(3)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", []byte("abcd"))
			mustSync(t, h)
			mustWrite(t, h, []byte("efgh"))

			disk := sim.MaterializeCrash(simenv.CrashPoint{
				Op:   tornWriteIndex(t, sim),
				Torn: tt.giveTorn,
				Mode: simenv.CrashPrefix,
			})

			if got := readFile(t, disk, "a"); !bytes.Equal(got, tt.want) {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaterializeCrash_NoneDropsTheTornWrite(t *testing.T) {
	t.Parallel()

	sim := simenv.New(4)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", []byte("abcd"))
	mustSync(t, h)
	mustSyncDir(t, fsys)
	mustWrite(t, h, []byte("efgh"))

	disk := sim.MaterializeCrash(simenv.CrashPoint{
		Op:   tornWriteIndex(t, sim),
		Torn: 2,
		Mode: simenv.CrashNone,
	})

	if got := readFile(t, disk, "a"); !bytes.Equal(got, []byte("abcd")) {
		t.Errorf("content = %q, want %q", got, "abcd")
	}
}

func TestMaterializeCrash_ScatterIsDeterministic(t *testing.T) {
	t.Parallel()

	sim := simenv.New(5)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", nil)
	mustSyncDir(t, fsys)
	mustWrite(t, h, sectorPattern(3))

	point := simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashScatter, ScatterSeed: 42}
	first := readFile(t, sim.MaterializeCrash(point), "a")
	second := readFile(t, sim.MaterializeCrash(point), "a")

	if !bytes.Equal(first, second) {
		t.Errorf("two materializations of seed 42 differ: %d and %d bytes", len(first), len(second))
	}
}

func TestMaterializeCrash_ScatterLosesAWholeSector(t *testing.T) {
	t.Parallel()

	sim := simenv.New(6)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", nil)
	mustSyncDir(t, fsys)
	pattern := sectorPattern(3)
	mustWrite(t, h, pattern)

	disk := sim.MaterializeCrash(simenv.CrashPoint{
		Op:          len(sim.Ops()),
		Mode:        simenv.CrashScatter,
		ScatterSeed: 42,
	})

	want := slices.Clone(pattern)
	clear(want[_sectorSize : 2*_sectorSize])

	if got := readFile(t, disk, "a"); !bytes.Equal(got, want) {
		t.Errorf("content differs from the expected hole at sector 1: %s", diff(got, want))
	}
}

func TestMaterializeCrash_DirectoryEntryNeedsSyncDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		giveSyncDir bool
		wantExists  bool
	}{
		{name: "synced file without a synced dirent is absent", giveSyncDir: false, wantExists: false},
		{name: "synced file with a synced dirent is present", giveSyncDir: true, wantExists: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(7)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", []byte("hello"))
			mustSync(t, h)
			if tt.giveSyncDir {
				mustSyncDir(t, fsys)
			}

			disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

			if got := hasFile(t, disk, "a"); got != tt.wantExists {
				t.Fatalf("file a present = %t, want %t", got, tt.wantExists)
			}
			if !tt.wantExists {
				return
			}
			if got := readFile(t, disk, "a"); !bytes.Equal(got, []byte("hello")) {
				t.Errorf("content = %q, want %q", got, "hello")
			}
		})
	}
}

func TestMaterializeCrash_RenameNeedsSyncDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		giveSyncDir bool
		wantName    string
	}{
		{name: "rename before the dir sync is lost", giveSyncDir: false, wantName: "a"},
		{name: "rename after the dir sync survives", giveSyncDir: true, wantName: "b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(8)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", []byte("hello"))
			mustSync(t, h)
			mustSyncDir(t, fsys)
			mustRename(t, fsys, "a", "b")
			if tt.giveSyncDir {
				mustSyncDir(t, fsys)
			}

			disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

			if got := listFiles(t, disk); !slices.Equal(got, []string{tt.wantName}) {
				t.Fatalf("list = %v, want [%s]", got, tt.wantName)
			}
			if got := readFile(t, disk, tt.wantName); !bytes.Equal(got, []byte("hello")) {
				t.Errorf("content = %q, want %q", got, "hello")
			}
		})
	}
}

func TestMaterializeCrash_IsIndependentOfTheLiveSim(t *testing.T) {
	t.Parallel()

	sim := simenv.New(9)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", []byte("hello"))
	mustSync(t, h)
	mustSyncDir(t, fsys)

	disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})

	mustWrite(t, h, []byte("MORE"))
	mustSync(t, h)
	createFile(t, fsys, "live", nil)
	mustSyncDir(t, fsys)

	if got := readFile(t, disk, "a"); !bytes.Equal(got, []byte("hello")) {
		t.Errorf("materialized content = %q, want %q", got, "hello")
	}
	if hasFile(t, disk, "live") {
		t.Error("a file created after materialization appeared on the materialized disk")
	}

	createFile(t, disk, "ghost", []byte("x"))
	if hasFile(t, fsys, "ghost") {
		t.Error("a file created on the materialized disk appeared on the live sim")
	}
	if got := readFile(t, fsys, "a"); !bytes.Equal(got, []byte("helloMORE")) {
		t.Errorf("live content = %q, want %q", got, "helloMORE")
	}

	if _, err := fsys.Lock(); err != nil {
		t.Fatalf("lock the live sim: %v", err)
	}
	if _, err := disk.Lock(); err != nil {
		t.Errorf("lock the materialized disk: %v", err)
	}
}

func TestMaterializeCrash_PartialNoSpaceWriteIsVolatile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode simenv.CrashMode
		want []byte
	}{
		{name: "prefix keeps the partial write", mode: simenv.CrashPrefix, want: []byte("abcdef")},
		{name: "none drops the partial write", mode: simenv.CrashNone, want: []byte("abcd")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(7)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", []byte("abcd"))
			mustSync(t, h)
			mustSyncDir(t, fsys)
			sim.SetDiskBudget(2)
			if _, err := h.Write([]byte("efgh")); !errors.Is(err, env.ErrNoSpace) {
				t.Fatalf("write error = %v, want %v", err, env.ErrNoSpace)
			}

			disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: tt.mode})

			if got := readFile(t, disk, "a"); !bytes.Equal(got, tt.want) {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}
