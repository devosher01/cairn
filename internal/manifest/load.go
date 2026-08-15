package manifest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/devosher01/cairn/internal/env"
)

func Load(dir env.FS) (State, bool, error) {
	f, err := dir.Open(FileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return State{}, false, nil
		}

		return State{}, false, fmt.Errorf("manifest: open %s: %w", FileName, err)
	}
	defer f.Close()

	raw, err := readAll(f)
	if err != nil {
		return State{}, true, err
	}
	s, err := Decode(raw)
	if err != nil {
		return State{}, true, err
	}

	return s, true, nil
}

func readAll(f env.File) ([]byte, error) {
	size, err := f.Size()
	if err != nil {
		return nil, fmt.Errorf("manifest: size %s: %w", FileName, err)
	}
	if size < 0 || size > _maxFileSize {
		return nil, fmt.Errorf("manifest: file of %d bytes: %w", size, ErrCorrupt)
	}

	raw := make([]byte, size)
	n, err := f.ReadAt(raw, 0)
	if n < len(raw) && err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("manifest: read %s: %w", FileName, err)
	}

	return raw[:n], nil
}
