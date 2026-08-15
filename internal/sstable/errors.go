package sstable

import (
	"errors"
	"fmt"
)

var ErrCorrupt = errors.New("sstable: corrupt")

func blockErr(offset int64, err error) error {
	return fmt.Errorf("%w: block at offset %d: %w", ErrCorrupt, offset, err)
}

func shortKeyErr(offset int64, length int) error {
	return fmt.Errorf("%w: block at offset %d holds a %d-byte key", ErrCorrupt, offset, length)
}
