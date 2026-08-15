package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	_dirPerm      = 0o755
	_iterationDir = "iteration%03d"
	_rootPattern  = "crashkill"
)

func runParent(base string, iterations int) error {
	if iterations <= 0 {
		return fmt.Errorf("crashkill: iterations must be positive, have %d", iterations)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("crashkill: locate the harness executable: %w", err)
	}
	root, temporary, err := campaignRoot(base)
	if err != nil {
		return err
	}
	fmt.Printf("root %s\n", root)

	totalAcked, totalVerified := 0, 0
	for i := range iterations {
		acked, verified, err := runIteration(self, root, i)
		if err != nil {
			return err
		}
		fmt.Printf("iteration %d acked %d verified %d\n", i, acked, verified)
		totalAcked += acked
		totalVerified += verified
	}
	fmt.Printf("iterations %d acked %d verified %d\n", iterations, totalAcked, totalVerified)

	if temporary {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("crashkill: remove %s: %w", root, err)
		}
	}

	return nil
}

func runIteration(self, root string, i int) (int, int, error) {
	dir := filepath.Join(root, fmt.Sprintf(_iterationDir, i))
	if err := os.MkdirAll(dir, _dirPerm); err != nil {
		return 0, 0, fmt.Errorf("crashkill: create %s: %w", dir, err)
	}
	acked, err := killWriter(self, dir)
	if err != nil {
		return 0, 0, fmt.Errorf("crashkill: iteration %d in %s: %w", i, dir, err)
	}
	verified, err := verifyDir(dir, acked)
	if err != nil {
		return 0, 0, fmt.Errorf("crashkill: iteration %d in %s: %w", i, dir, err)
	}

	return len(acked), verified, nil
}

func campaignRoot(base string) (string, bool, error) {
	if base == "" {
		root, err := os.MkdirTemp("", _rootPattern)
		if err != nil {
			return "", false, fmt.Errorf("crashkill: create a temporary root: %w", err)
		}

		return root, true, nil
	}
	if err := os.MkdirAll(base, _dirPerm); err != nil {
		return "", false, fmt.Errorf("crashkill: create %s: %w", base, err)
	}

	return base, false, nil
}
