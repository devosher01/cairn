package simenv

import (
	"slices"
	"sync"
	"time"

	"github.com/devosher01/cairn/internal/env"
)

const _baseUnixSec = 1_700_000_000

type Clock struct {
	mu      *sync.Mutex
	now     time.Time
	tickers []*ticker
}

var _ env.Clock = (*Clock)(nil)

func newClock(mu *sync.Mutex) *Clock {
	return &Clock{mu: mu, now: time.Unix(_baseUnixSec, 0)}
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *Clock) NewTicker(d time.Duration) env.Ticker {
	if d <= 0 {
		panic("simenv: non-positive ticker period")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	t := &ticker{
		clock:  c,
		period: d,
		next:   c.now.Add(d),
		c:      make(chan time.Time, 1),
	}
	c.tickers = append(c.tickers, t)
	return t
}

func (c *Clock) Advance(d time.Duration) {
	if d < 0 {
		panic("simenv: negative clock advance")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
	for _, t := range c.tickers {
		t.fire(c.now)
	}
}

type ticker struct {
	clock  *Clock
	period time.Duration
	next   time.Time
	c      chan time.Time
}

var _ env.Ticker = (*ticker)(nil)

func (t *ticker) C() <-chan time.Time {
	return t.c
}

func (t *ticker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	t.clock.tickers = slices.DeleteFunc(t.clock.tickers, func(other *ticker) bool {
		return other == t
	})
}

func (t *ticker) fire(now time.Time) {
	for !t.next.After(now) {
		select {
		case t.c <- t.next:
		default:
		}
		t.next = t.next.Add(t.period)
	}
}
