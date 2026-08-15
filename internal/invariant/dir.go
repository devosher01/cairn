package invariant

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devosher01/cairn/internal/env"
)

const (
	_sstSuffix       = ".sst"
	_walSuffix       = ".wal"
	_manifestTmpName = "MANIFEST.tmp"
)

type dirFiles struct {
	ssts map[uint64]string
	wals map[uint64]string
	tmp  bool
}

func scanDir(fs env.FS) (dirFiles, error) {
	names, err := fs.List()
	if err != nil {
		return dirFiles{}, fmt.Errorf("invariant: list directory: %w", err)
	}

	out := dirFiles{ssts: make(map[uint64]string), wals: make(map[uint64]string)}
	for _, name := range names {
		if name == _manifestTmpName {
			out.tmp = true

			continue
		}
		if num, ok := fileNumber(name, _sstSuffix); ok {
			out.ssts[num] = name

			continue
		}
		if num, ok := fileNumber(name, _walSuffix); ok {
			out.wals[num] = name
		}
	}

	return out, nil
}

func (d dirFiles) holdsData() bool {
	return len(d.ssts) > 0 || len(d.wals) > 0
}

func sstName(num uint64) string {
	return fmt.Sprintf("%06d%s", num, _sstSuffix)
}

func fileNumber(name, suffix string) (uint64, bool) {
	base, ok := strings.CutSuffix(name, suffix)
	if !ok {
		return 0, false
	}
	num, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, false
	}

	return num, true
}
