package invariant

import (
	"errors"
	"fmt"
)

var ErrViolated = errors.New("invariant: violated")

func violationf(tag, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrViolated, tag, fmt.Sprintf(format, args...))
}
