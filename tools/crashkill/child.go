package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/devosher01/cairn"
)

const _childMemtableSize int64 = 32 << 10

func runChild(dir string) error {
	if dir == "" {
		return errors.New("crashkill: the child requires -dir")
	}
	db, err := cairn.Open(dir, &cairn.Options{
		Sync:         cairn.SyncAlways,
		MemtableSize: _childMemtableSize,
	})
	if err != nil {
		return fmt.Errorf("crashkill: open %s: %w", dir, err)
	}
	for i := 0; ; i++ {
		key := keyAt(i)
		if err := db.Put([]byte(key), valueAt(i)); err != nil {
			return fmt.Errorf("crashkill: put %s: %w", key, err)
		}
		if _, err := os.Stdout.WriteString(key + "\n"); err != nil {
			return fmt.Errorf("crashkill: acknowledge %s: %w", key, err)
		}
	}
}
