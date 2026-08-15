package simenv_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/simenv"
)

const _sectorSize = 512

func createFile(t *testing.T, fsys env.FS, name string, data []byte) env.File {
	t.Helper()

	h, err := fsys.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if len(data) > 0 {
		mustWrite(t, h, data)
	}
	return h
}

func mustWrite(t *testing.T, h env.File, data []byte) {
	t.Helper()

	n, err := h.Write(data)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("write returned %d bytes, want %d", n, len(data))
	}
}

func mustSync(t *testing.T, h env.File) {
	t.Helper()

	if err := h.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func mustSyncDir(t *testing.T, fsys env.FS) {
	t.Helper()

	if err := fsys.SyncDir(); err != nil {
		t.Fatalf("sync dir: %v", err)
	}
}

func mustRename(t *testing.T, fsys env.FS, oldname, newname string) {
	t.Helper()

	if err := fsys.Rename(oldname, newname); err != nil {
		t.Fatalf("rename %s to %s: %v", oldname, newname, err)
	}
}

func mustRemove(t *testing.T, fsys env.FS, name string) {
	t.Helper()

	if err := fsys.Remove(name); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
}

func assertSize(t *testing.T, h env.File, want int64) {
	t.Helper()

	got, err := h.Size()
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	if got != want {
		t.Errorf("size = %d, want %d", got, want)
	}
}

func readFile(t *testing.T, fsys env.FS, name string) []byte {
	t.Helper()

	h, err := fsys.Open(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer func() { _ = h.Close() }()

	size, err := h.Size()
	if err != nil {
		t.Fatalf("size %s: %v", name, err)
	}
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	if _, err := h.ReadAt(buf, 0); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return buf
}

func listFiles(t *testing.T, fsys env.FS) []string {
	t.Helper()

	names, err := fsys.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return names
}

func hasFile(t *testing.T, fsys env.FS, name string) bool {
	t.Helper()

	return slices.Contains(listFiles(t, fsys), name)
}

func tornWriteIndex(t *testing.T, sim *simenv.Sim) int {
	t.Helper()

	for i, op := range slices.Backward(sim.Ops()) {
		if op.Kind == simenv.OpWrite {
			return i
		}
	}
	t.Fatal("no write op to tear")
	return 0
}

func sectorPattern(sectors int) []byte {
	data := make([]byte, sectors*_sectorSize)
	for i := range data {
		data[i] = byte(i%251) + 1
	}
	return data
}

func diff(got, want []byte) string {
	if len(got) != len(want) {
		return fmt.Sprintf("length %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Sprintf("first difference at byte %d", i)
		}
	}
	return "no difference"
}

func wantPanic(t *testing.T, action string, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Errorf("%s: want panic, got none", action)
		}
	}()
	fn()
}
