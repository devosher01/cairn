package cairn

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	_walSuffix       = ".wal"
	_sstSuffix       = ".sst"
	_manifestTmpName = "MANIFEST.tmp"
)

func walName(num uint64) string {
	return fmt.Sprintf("%06d%s", num, _walSuffix)
}

func walNumber(name string) (uint64, bool) {
	return fileNumber(name, _walSuffix)
}

func sstName(num uint64) string {
	return fmt.Sprintf("%06d%s", num, _sstSuffix)
}

func sstNumber(name string) (uint64, bool) {
	return fileNumber(name, _sstSuffix)
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

func sortWALs(names []string) {
	slices.SortFunc(names, func(a, b string) int {
		na, _ := walNumber(a)
		nb, _ := walNumber(b)
		return cmp.Compare(na, nb)
	})
}
