package sstable

import (
	"errors"
	"testing"

	"github.com/devosher01/cairn/internal/keys"
)

func FuzzOpen(f *testing.F) {
	golden := goldenTable(f)
	f.Add(golden)
	f.Add(golden[:len(golden)/2])
	f.Add(make([]byte, _footerSize))

	f.Fuzz(func(t *testing.T, give []byte) {
		table, err := Open(&fakeFile{data: give}, int64(len(give)))
		if err != nil {
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Open error = %v, want %v", err, ErrCorrupt)
			}

			return
		}

		for _, user := range []string{"", "alpha", "key0007"} {
			value, kind, ok, err := table.Get([]byte(user), keys.MaxSeq)
			if err != nil && !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Get(%q) error = %v, want nil or %v", user, err, ErrCorrupt)
			}
			if ok && !kind.Valid() {
				t.Fatalf("Get(%q) = (%q, kind %d, true), want a valid kind", user, value, kind)
			}
		}

		for ikey := range table.All() {
			if !validIKey(ikey) {
				t.Fatalf("All yielded a %d-byte key", len(ikey))
			}
		}
		if err := table.AllErr(); err != nil && !errors.Is(err, ErrCorrupt) {
			t.Fatalf("AllErr = %v, want nil or %v", err, ErrCorrupt)
		}
	})
}
