package invariant

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
	"github.com/devosher01/cairn/internal/sstable"
)

func checkTables(fs env.FS, files dirFiles, s manifest.State) error {
	for level, tables := range s.Levels {
		for _, t := range tables {
			if err := checkTable(fs, files.ssts[t.FileNum], level, t, s.LastSeq); err != nil {
				return err
			}
		}
	}

	return nil
}

func checkTable(fs env.FS, name string, level int, t manifest.Table, lastSeq keys.Seq) error {
	f, err := fs.Open(name)
	if err != nil {
		return fmt.Errorf("invariant: open %s: %w", name, err)
	}
	tbl, err := sstable.Open(f, int64(t.Size))
	if err != nil {
		_ = f.Close()
		if !errors.Is(err, sstable.ErrCorrupt) {
			return fmt.Errorf("invariant: open table %s: %w", name, err)
		}

		return violationf("I3", "%s does not open: %v", name, err)
	}
	defer func() {
		_ = tbl.Close()
	}()

	w := walk{name: name, level: level, table: t, lastSeq: lastSeq, sample: newSampler(t.EntryCount)}
	for ikey := range tbl.All() {
		if err := w.visit(ikey); err != nil {
			return err
		}
	}
	if err := tbl.AllErr(); err != nil {
		if !errors.Is(err, sstable.ErrCorrupt) {
			return fmt.Errorf("invariant: read table %s: %w", name, err)
		}

		return violationf("I4", "%s does not read back: %v", name, err)
	}
	if err := w.done(); err != nil {
		return err
	}

	return checkFilter(tbl, name, w.sample.sampled())
}

type walk struct {
	name    string
	level   int
	table   manifest.Table
	lastSeq keys.Seq

	sample   sampler
	first    []byte
	prev     []byte
	prevUser []byte
	count    uint64
}

func (w *walk) visit(ikey []byte) error {
	if w.count == 0 {
		w.first = bytes.Clone(ikey)
	} else if keys.Compare(w.prev, ikey) >= 0 {
		return violationf("I3", "%s holds %q at position %d, at or below its predecessor %q",
			w.name, ikey, w.count, w.prev)
	}

	seq, _ := keys.Trailer(ikey)
	if seq > w.lastSeq {
		return violationf("I6", "%s holds %q at sequence %d, above the manifest last sequence %d",
			w.name, ikey, seq, w.lastSeq)
	}

	user := keys.UserKey(ikey)
	if w.count == 0 || !bytes.Equal(user, w.prevUser) {
		w.sample.add(user)
	}
	w.prevUser = append(w.prevUser[:0], user...)
	w.prev = append(w.prev[:0], ikey...)
	w.count++

	return nil
}

func (w *walk) done() error {
	if w.count != w.table.EntryCount {
		return violationf("I3", "%s holds %d entries, level %d records %d",
			w.name, w.count, w.level, w.table.EntryCount)
	}
	if !bytes.Equal(w.first, w.table.Smallest) {
		return violationf("I3", "%s starts at %q, level %d records smallest %q",
			w.name, w.first, w.level, w.table.Smallest)
	}
	if !bytes.Equal(w.prev, w.table.Largest) {
		return violationf("I3", "%s ends at %q, level %d records largest %q",
			w.name, w.prev, w.level, w.table.Largest)
	}

	return nil
}

func checkFilter(tbl *sstable.Table, name string, sampled [][]byte) error {
	for _, user := range sampled {
		_, _, ok, err := tbl.Get(user, keys.MaxSeq)
		if err != nil {
			if !errors.Is(err, sstable.ErrCorrupt) {
				return fmt.Errorf("invariant: look up %q in %s: %w", user, name, err)
			}

			return violationf("I4", "%s fails the lookup of its own key %q: %v", name, user, err)
		}
		if !ok {
			return violationf("I5", "%s does not find its own key %q", name, user)
		}
	}

	return nil
}
