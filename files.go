package cairn

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/devosher01/cairn/internal/env"
)

const _walSuffix = ".wal"

func walName(num uint64) string {
	return fmt.Sprintf("%06d%s", num, _walSuffix)
}

func walNumber(name string) (uint64, bool) {
	base, ok := strings.CutSuffix(name, _walSuffix)
	if !ok {
		return 0, false
	}
	num, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, false
	}
	return num, true
}

func walFiles(fs env.FS) ([]string, uint64, error) {
	names, err := fs.List()
	if err != nil {
		return nil, 0, err
	}
	type walFile struct {
		num  uint64
		name string
	}
	var wals []walFile
	var maxNum uint64
	for _, name := range names {
		num, ok := walNumber(name)
		if !ok {
			continue
		}
		wals = append(wals, walFile{num: num, name: name})
		maxNum = max(maxNum, num)
	}
	slices.SortFunc(wals, func(a, b walFile) int {
		return cmp.Compare(a.num, b.num)
	})
	out := make([]string, len(wals))
	for i, w := range wals {
		out[i] = w.name
	}
	return out, maxNum, nil
}
