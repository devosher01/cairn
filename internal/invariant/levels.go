package invariant

import (
	"bytes"

	"github.com/devosher01/cairn/internal/keys"
	"github.com/devosher01/cairn/internal/manifest"
)

func checkLevels(s manifest.State) error {
	for level := 1; level < manifest.NumLevels; level++ {
		tables := s.Levels[level]
		for i := 1; i < len(tables); i++ {
			prev, next := tables[i-1], tables[i]
			if keys.Compare(prev.Smallest, next.Smallest) >= 0 {
				return violationf("I2", "level %d tables %d and %d are out of order: %q does not precede %q",
					level, i-1, i, prev.Smallest, next.Smallest)
			}
			if bytes.Compare(keys.UserKey(prev.Largest), keys.UserKey(next.Smallest)) >= 0 {
				return violationf("I2", "level %d tables %d and %d overlap: %s ends at user key %q, %s starts at %q",
					level, i-1, i,
					sstName(prev.FileNum), keys.UserKey(prev.Largest),
					sstName(next.FileNum), keys.UserKey(next.Smallest))
			}
		}
	}

	return nil
}
