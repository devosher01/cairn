package manifest

import (
	"fmt"

	"github.com/devosher01/cairn/internal/env"
)

func Install(dir env.FS, s State) error {
	if err := writeTmp(dir, Encode(s)); err != nil {
		return err
	}
	if err := dir.Rename(_tmpFileName, FileName); err != nil {
		return fmt.Errorf("manifest: rename %s: %w", _tmpFileName, err)
	}
	if err := dir.SyncDir(); err != nil {
		return fmt.Errorf("manifest: sync directory: %w", err)
	}

	return nil
}

func writeTmp(dir env.FS, raw []byte) error {
	f, err := dir.Create(_tmpFileName)
	if err != nil {
		return fmt.Errorf("manifest: create %s: %w", _tmpFileName, err)
	}
	if err := writeSync(f, raw); err != nil {
		_ = f.Close()

		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("manifest: close %s: %w", _tmpFileName, err)
	}

	return nil
}

func writeSync(f env.File, raw []byte) error {
	n, err := f.Write(raw)
	if err != nil {
		return fmt.Errorf("manifest: write %s: %w", _tmpFileName, err)
	}
	if n != len(raw) {
		return fmt.Errorf("manifest: wrote %d of %d bytes to %s", n, len(raw), _tmpFileName)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("manifest: sync %s: %w", _tmpFileName, err)
	}

	return nil
}
