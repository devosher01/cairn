package simenv_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

var errInjected = errors.New("simenv_test: injected")

func TestFS_ReadsOwnUnsyncedWrites(t *testing.T) {
	t.Parallel()

	sim := simenv.New(1)
	fsys := sim.Env().FS
	w := createFile(t, fsys, "a", []byte("hello"))

	r, err := fsys.Open("a")
	if err != nil {
		t.Fatalf("open a: %v", err)
	}
	assertSize(t, r, 5)

	buf := make([]byte, 5)
	if _, err := r.ReadAt(buf, 0); err != nil {
		t.Fatalf("read a: %v", err)
	}
	if !bytes.Equal(buf, []byte("hello")) {
		t.Errorf("content = %q, want %q", buf, "hello")
	}

	mustWrite(t, w, []byte("!"))
	assertSize(t, r, 6)

	n, err := r.ReadAt(make([]byte, 4), 4)
	if n != 2 || !errors.Is(err, io.EOF) {
		t.Errorf("read past end = (%d, %v), want (2, EOF)", n, err)
	}
}

func TestFS_ListIsSorted(t *testing.T) {
	t.Parallel()

	sim := simenv.New(2)
	fsys := sim.Env().FS
	for _, name := range []string{"c", "a", "b"} {
		createFile(t, fsys, name, nil)
	}

	if got := listFiles(t, fsys); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("list = %v, want [a b c]", got)
	}
}

func TestFS_RemoveDropsTheEntry(t *testing.T) {
	t.Parallel()

	sim := simenv.New(3)
	fsys := sim.Env().FS
	createFile(t, fsys, "a", nil)
	createFile(t, fsys, "b", nil)
	mustRemove(t, fsys, "a")

	if got := listFiles(t, fsys); !slices.Equal(got, []string{"b"}) {
		t.Errorf("list = %v, want [b]", got)
	}
	if _, err := fsys.Open("a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open removed file = %v, want ErrNotExist", err)
	}
}

func TestFS_LockIsExclusiveUntilClosed(t *testing.T) {
	t.Parallel()

	sim := simenv.New(4)
	fsys := sim.Env().FS

	held, err := fsys.Lock()
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := fsys.Lock(); !errors.Is(err, env.ErrLocked) {
		t.Errorf("second lock = %v, want ErrLocked", err)
	}
	if err := held.Close(); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	again, err := fsys.Lock()
	if err != nil {
		t.Errorf("lock after release: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Errorf("release second lock: %v", err)
	}
}

func TestFS_OpenMissingWrapsErrNotExist(t *testing.T) {
	t.Parallel()

	sim := simenv.New(5)
	if _, err := sim.Env().FS.Open("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open missing = %v, want ErrNotExist", err)
	}
}

func TestFS_RenameMissingSourceFailsAndIsLogged(t *testing.T) {
	t.Parallel()

	sim := simenv.New(6)
	err := sim.Env().FS.Rename("missing", "other")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("rename missing = %v, want ErrNotExist", err)
	}

	want := simenv.Op{Kind: simenv.OpRename, Name: "missing", To: "other", Failed: true}
	if got := sim.Ops(); len(got) != 1 || got[0] != want {
		t.Errorf("ops = %+v, want [%+v]", got, want)
	}
}

func TestFS_RenameOverwritesDestination(t *testing.T) {
	t.Parallel()

	sim := simenv.New(7)
	fsys := sim.Env().FS
	createFile(t, fsys, "a", []byte("one"))
	createFile(t, fsys, "b", []byte("two"))
	mustRename(t, fsys, "a", "b")

	if got := listFiles(t, fsys); !slices.Equal(got, []string{"b"}) {
		t.Errorf("list = %v, want [b]", got)
	}
	if got := readFile(t, fsys, "b"); !bytes.Equal(got, []byte("one")) {
		t.Errorf("content of b = %q, want %q", got, "one")
	}
}

func TestFS_DiskBudgetCapsWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		giveBudget int64
		giveData   []byte
		wantN      int
		wantErr    error
		wantOp     simenv.Op
	}{
		{
			name:       "write within the budget succeeds",
			giveBudget: 8,
			giveData:   []byte("abcd"),
			wantN:      4,
			wantErr:    nil,
			wantOp:     simenv.Op{Kind: simenv.OpWrite, Name: "a", Len: 4},
		},
		{
			name:       "write beyond the budget keeps the prefix that fits",
			giveBudget: 2,
			giveData:   []byte("abcd"),
			wantN:      2,
			wantErr:    env.ErrNoSpace,
			wantOp:     simenv.Op{Kind: simenv.OpWrite, Name: "a", Len: 2, Failed: true},
		},
		{
			name:       "write with an exhausted budget keeps nothing",
			giveBudget: 0,
			giveData:   []byte("abcd"),
			wantN:      0,
			wantErr:    env.ErrNoSpace,
			wantOp:     simenv.Op{Kind: simenv.OpWrite, Name: "a", Len: 0, Failed: true},
		},
		{
			name:       "a negative budget restores unlimited space",
			giveBudget: -1,
			giveData:   []byte("abcd"),
			wantN:      4,
			wantErr:    nil,
			wantOp:     simenv.Op{Kind: simenv.OpWrite, Name: "a", Len: 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(8)
			fsys := sim.Env().FS
			h := createFile(t, fsys, "a", nil)
			sim.SetDiskBudget(tt.giveBudget)

			n, err := h.Write(tt.giveData)
			if n != tt.wantN {
				t.Errorf("write returned %d, want %d", n, tt.wantN)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("write error = %v, want %v", err, tt.wantErr)
			}
			assertSize(t, h, int64(tt.wantN))

			if got := sim.Ops()[1]; got != tt.wantOp {
				t.Errorf("op = %+v, want %+v", got, tt.wantOp)
			}
		})
	}
}

func TestFS_DiskBudgetSpansWrites(t *testing.T) {
	t.Parallel()

	sim := simenv.New(9)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", nil)
	sim.SetDiskBudget(6)

	mustWrite(t, h, []byte("abcd"))

	n, err := h.Write([]byte("efgh"))
	if n != 2 || !errors.Is(err, env.ErrNoSpace) {
		t.Errorf("second write = (%d, %v), want (2, ErrNoSpace)", n, err)
	}
	n, err = h.Write([]byte("i"))
	if n != 0 || !errors.Is(err, env.ErrNoSpace) {
		t.Errorf("third write = (%d, %v), want (0, ErrNoSpace)", n, err)
	}
	if got := readFile(t, fsys, "a"); !bytes.Equal(got, []byte("abcdef")) {
		t.Errorf("content = %q, want %q", got, "abcdef")
	}
}

func TestFS_InjectedFaultFailsWriteWithoutMutating(t *testing.T) {
	t.Parallel()

	sim := simenv.New(10)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", nil)
	sim.InjectFault(1, errInjected)

	n, err := h.Write([]byte("hello"))
	if n != 0 || !errors.Is(err, errInjected) {
		t.Errorf("faulted write = (%d, %v), want (0, injected)", n, err)
	}
	assertSize(t, h, 0)

	mustWrite(t, h, []byte("ok"))

	want := []simenv.Op{
		{Kind: simenv.OpCreate, Name: "a"},
		{Kind: simenv.OpWrite, Name: "a", Failed: true},
		{Kind: simenv.OpWrite, Name: "a", Len: 2},
	}
	if got := sim.Ops(); !slices.Equal(got, want) {
		t.Errorf("ops = %+v, want %+v", got, want)
	}
	if got := readFile(t, fsys, "a"); !bytes.Equal(got, []byte("ok")) {
		t.Errorf("content = %q, want %q", got, "ok")
	}
}

func TestFS_InjectedFaultFailsSyncWithoutPromoting(t *testing.T) {
	t.Parallel()

	sim := simenv.New(11)
	fsys := sim.Env().FS
	h := createFile(t, fsys, "a", nil)
	mustSyncDir(t, fsys)
	mustWrite(t, h, []byte("hello"))
	sim.InjectFault(3, errInjected)

	if err := h.Sync(); !errors.Is(err, errInjected) {
		t.Errorf("faulted sync = %v, want injected", err)
	}
	if got := sim.Ops()[3]; !got.Failed || got.Kind != simenv.OpSync {
		t.Errorf("op = %+v, want a failed sync", got)
	}

	disk := sim.MaterializeCrash(simenv.CrashPoint{Op: len(sim.Ops()), Mode: simenv.CrashNone})
	if !hasFile(t, disk, "a") {
		t.Fatal("file a is absent after the crash")
	}
	if got := readFile(t, disk, "a"); got != nil {
		t.Errorf("content = %q, want empty", got)
	}
}

func TestFile_ReadAtOnWriteHandlePanics(t *testing.T) {
	t.Parallel()

	h := createFile(t, simenv.New(12).Env().FS, "a", nil)
	wantPanic(t, "read at on a write handle", func() {
		_, _ = h.ReadAt(make([]byte, 1), 0)
	})
}

func TestFile_WriteOnReadHandlePanics(t *testing.T) {
	t.Parallel()

	fsys := simenv.New(13).Env().FS
	createFile(t, fsys, "a", nil)
	r, err := fsys.Open("a")
	if err != nil {
		t.Fatalf("open a: %v", err)
	}

	wantPanic(t, "write on a read handle", func() {
		_, _ = r.Write([]byte("x"))
	})
	wantPanic(t, "sync on a read handle", func() {
		_ = r.Sync()
	})
}
