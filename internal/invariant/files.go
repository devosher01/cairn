package invariant

import (
	"fmt"
	"maps"
	"slices"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/manifest"
)

func checkReferenced(fs env.FS, files dirFiles, s manifest.State) error {
	for level, tables := range s.Levels {
		for _, t := range tables {
			name, ok := files.ssts[t.FileNum]
			if !ok {
				return violationf("I1", "level %d references %s, absent from the directory", level, sstName(t.FileNum))
			}
			size, err := fileSize(fs, name)
			if err != nil {
				return err
			}
			if size != int64(t.Size) {
				return violationf("I1", "%s holds %d bytes, level %d records %d", name, size, level, t.Size)
			}
		}
	}

	return nil
}

func checkFileNumbers(files dirFiles, s manifest.State, m mode) error {
	for level, tables := range s.Levels {
		for _, t := range tables {
			if t.FileNum >= s.NextFileNum {
				return violationf("I7", "level %d references %s at or above the next file number %d",
					level, sstName(t.FileNum), s.NextFileNum)
			}
			if wal, ok := files.wals[t.FileNum]; ok {
				return violationf("I7", "file number %d serves both %s and %s", t.FileNum, sstName(t.FileNum), wal)
			}
		}
	}
	if m == _modeCrashDisk {
		return nil
	}

	for _, num := range slices.Sorted(maps.Keys(files.ssts)) {
		if num >= s.NextFileNum {
			return violationf("I7", "%s sits at or above the next file number %d", files.ssts[num], s.NextFileNum)
		}
	}

	return nil
}

func checkOrphans(files dirFiles, s manifest.State) error {
	referenced := referencedNums(s)
	for _, num := range slices.Sorted(maps.Keys(files.ssts)) {
		if !referenced[num] {
			return violationf("I8", "%s is an orphan: no manifest level references it", files.ssts[num])
		}
	}
	for _, num := range slices.Sorted(maps.Keys(files.wals)) {
		if num < s.OldestWAL {
			return violationf("I8", "%s precedes the oldest live write-ahead log %d", files.wals[num], s.OldestWAL)
		}
	}
	if files.tmp {
		return violationf("I8", "%s survives in the directory", _manifestTmpName)
	}

	return nil
}

func referencedNums(s manifest.State) map[uint64]bool {
	out := make(map[uint64]bool)
	for _, tables := range s.Levels {
		for _, t := range tables {
			out[t.FileNum] = true
		}
	}

	return out
}

func fileSize(fs env.FS, name string) (int64, error) {
	f, err := fs.Open(name)
	if err != nil {
		return 0, fmt.Errorf("invariant: open %s: %w", name, err)
	}
	defer func() {
		_ = f.Close()
	}()

	size, err := f.Size()
	if err != nil {
		return 0, fmt.Errorf("invariant: size %s: %w", name, err)
	}

	return size, nil
}
