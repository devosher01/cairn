package osenv_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/osenv"
)

func TestNew_RoundTripsAFile(t *testing.T) {
	t.Parallel()

	fsys := newFS(t, t.TempDir())

	w, err := fsys.Create("000001.wal")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, err := w.Size(); err != nil || got != 5 {
		t.Fatalf("size = (%d, %v), want (5, nil)", got, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r, err := fsys.Open("000001.wal")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = r.Close() }()

	size, err := r.Size()
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, 0); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, []byte("hello")) {
		t.Errorf("content = %q, want %q", buf, "hello")
	}
}

func TestFS_RenameAndRemoveShowInList(t *testing.T) {
	t.Parallel()

	fsys := newFS(t, t.TempDir())
	create(t, fsys, "MANIFEST.tmp")
	create(t, fsys, "000002.sst")

	if err := fsys.Rename("MANIFEST.tmp", "MANIFEST"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := list(t, fsys); !slices.Equal(got, []string{"000002.sst", "MANIFEST"}) {
		t.Errorf("list = %v, want [000002.sst MANIFEST]", got)
	}

	if err := fsys.Remove("000002.sst"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := list(t, fsys); !slices.Equal(got, []string{"MANIFEST"}) {
		t.Errorf("list = %v, want [MANIFEST]", got)
	}
}

func TestFS_SyncDirSucceeds(t *testing.T) {
	t.Parallel()

	fsys := newFS(t, t.TempDir())
	create(t, fsys, "MANIFEST")

	if err := fsys.SyncDir(); err != nil {
		t.Errorf("sync dir: %v", err)
	}
}

func TestFS_LockIsExclusivePerDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := newFS(t, dir)
	second := newFS(t, dir)

	held, err := first.Lock()
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := second.Lock(); !errors.Is(err, env.ErrLocked) {
		t.Errorf("second lock = %v, want ErrLocked", err)
	}
	if err := held.Close(); err != nil {
		t.Fatalf("release: %v", err)
	}

	again, err := second.Lock()
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Errorf("release second lock: %v", err)
	}
}

func newFS(t *testing.T, dir string) env.FS {
	t.Helper()

	e, err := osenv.New(dir)
	if err != nil {
		t.Fatalf("new osenv: %v", err)
	}
	return e.FS
}

func create(t *testing.T, fsys env.FS, name string) {
	t.Helper()

	h, err := fsys.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("close %s: %v", name, err)
	}
}

func list(t *testing.T, fsys env.FS) []string {
	t.Helper()

	names, err := fsys.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return names
}
