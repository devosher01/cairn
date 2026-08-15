package invariant

import (
	"errors"
	"fmt"

	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/manifest"
)

type mode uint8

const (
	_modeStrict mode = iota + 1
	_modeCrashDisk
)

func Check(fs env.FS) error {
	return check(fs, _modeStrict)
}

func CheckCrashDisk(fs env.FS) error {
	return check(fs, _modeCrashDisk)
}

func check(fs env.FS, m mode) error {
	files, err := scanDir(fs)
	if err != nil {
		return err
	}

	state, exists, err := manifest.Load(fs)
	if err != nil {
		if !errors.Is(err, manifest.ErrCorrupt) {
			return fmt.Errorf("invariant: load %s: %w", manifest.FileName, err)
		}

		return violationf("I1", "%s does not decode: %v", manifest.FileName, err)
	}
	if !exists {
		if files.holdsData() {
			return violationf("I1", "no %s alongside %d sstables and %d write-ahead logs",
				manifest.FileName, len(files.ssts), len(files.wals))
		}

		return nil
	}

	if err := checkReferenced(fs, files, state); err != nil {
		return err
	}
	if err := checkLevels(state); err != nil {
		return err
	}
	if err := checkTables(fs, files, state); err != nil {
		return err
	}
	if err := checkFileNumbers(files, state, m); err != nil {
		return err
	}
	if m == _modeCrashDisk {
		return nil
	}

	return checkOrphans(files, state)
}
