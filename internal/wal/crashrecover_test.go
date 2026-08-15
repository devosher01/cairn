package wal_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/wal"
)

func recoverDisk(t *testing.T, disk *simenv.FS, at string) [][]byte {
	t.Helper()

	names, err := disk.List()
	if err != nil {
		t.Fatalf("%s: List returned error: %v", at, err)
	}
	var wals []string
	for _, name := range names {
		if strings.HasSuffix(name, ".wal") {
			wals = append(wals, name)
		}
	}
	slices.Sort(wals)

	var out [][]byte
	for _, name := range wals {
		f, err := disk.Open(name)
		if err != nil {
			t.Fatalf("%s: Open %s returned error: %v", at, name, err)
		}
		size, err := f.Size()
		if err != nil {
			t.Fatalf("%s: Size of %s returned error: %v", at, name, err)
		}
		_, err = wal.Replay(f, size, func(payload []byte) error {
			out = append(out, slices.Clone(payload))

			return nil
		})
		if err != nil {
			t.Fatalf("%s: Replay of %s returned error: %v", at, name, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("%s: Close of %s returned error: %v", at, name, err)
		}
	}

	return out
}

func describePoint(p simenv.CrashPoint) string {
	return fmt.Sprintf("crash at op %d torn %d mode %s seed %d", p.Op, p.Torn, modeName(p.Mode), p.ScatterSeed)
}

func modeName(mode simenv.CrashMode) string {
	switch mode {
	case simenv.CrashNone:
		return "none"
	case simenv.CrashPrefix:
		return "prefix"
	case simenv.CrashScatter:
		return "scatter"
	default:
		return fmt.Sprintf("unknown(%d)", mode)
	}
}
