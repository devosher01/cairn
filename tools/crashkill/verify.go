package main

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/devosher01/cairn"
	"github.com/devosher01/cairn/internal/env/osenv"
	"github.com/devosher01/cairn/internal/invariant"
)

func verifyDir(dir string, acked []string) (int, error) {
	verified, err := verifyAcked(dir, acked)
	if err != nil {
		return 0, err
	}
	if err := verifyInvariants(dir); err != nil {
		return 0, err
	}

	return verified, nil
}

func verifyAcked(dir string, acked []string) (int, error) {
	db, err := cairn.Open(dir, nil)
	if err != nil {
		return 0, fmt.Errorf("reopen: %w", err)
	}
	verified, readErr := readAcked(db, acked)
	if closeErr := db.Close(); closeErr != nil {
		return verified, errors.Join(readErr, fmt.Errorf("close: %w", closeErr))
	}

	return verified, readErr
}

func readAcked(db *cairn.DB, acked []string) (int, error) {
	for n, key := range acked {
		i, err := indexAt(key)
		if err != nil {
			return n, err
		}
		value, err := db.Get([]byte(key))
		if err != nil {
			return n, fmt.Errorf("get acked key %s: %w", key, err)
		}
		if want := valueAt(i); !bytes.Equal(value, want) {
			return n, fmt.Errorf("acked key %s holds %x, want %x", key, value, want)
		}
	}

	return len(acked), nil
}

func verifyInvariants(dir string) error {
	sandbox, err := osenv.New(dir)
	if err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	if err := invariant.Check(sandbox.FS); err != nil {
		return fmt.Errorf("invariants: %w", err)
	}

	return nil
}
