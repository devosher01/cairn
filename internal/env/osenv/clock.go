package osenv

import (
	"time"

	"github.com/devosher01/cairn/internal/env"
)

type clock struct{}

var _ env.Clock = clock{}

func (clock) Now() time.Time {
	return time.Now()
}

func (clock) NewTicker(d time.Duration) env.Ticker {
	return &ticker{t: time.NewTicker(d)}
}

type ticker struct {
	t *time.Ticker
}

var _ env.Ticker = (*ticker)(nil)

func (t *ticker) C() <-chan time.Time {
	return t.t.C
}

func (t *ticker) Stop() {
	t.t.Stop()
}
