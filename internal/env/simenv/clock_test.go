package simenv_test

import (
	"testing"
	"time"

	"github.com/devosher01/cairn/internal/env/simenv"
)

func TestClock_AdvanceMovesNow(t *testing.T) {
	t.Parallel()

	clock := simenv.New(1).Clock()
	base := clock.Now()
	clock.Advance(90 * time.Second)

	if got := clock.Now().Sub(base); got != 90*time.Second {
		t.Errorf("elapsed = %v, want 1m30s", got)
	}
}

func TestClock_TickerFiresOncePerPeriod(t *testing.T) {
	t.Parallel()

	const periods = 5

	clock := simenv.New(2).Clock()
	base := clock.Now()
	ticker := clock.NewTicker(time.Second)
	defer ticker.Stop()

	for i := range periods {
		clock.Advance(time.Second)
		select {
		case got := <-ticker.C():
			if want := base.Add(time.Duration(i+1) * time.Second); !got.Equal(want) {
				t.Errorf("tick %d = %v, want %v", i, got, want)
			}
		default:
			t.Fatalf("tick %d never arrived", i)
		}
	}

	select {
	case got := <-ticker.C():
		t.Errorf("unexpected extra tick at %v", got)
	default:
	}
}

func TestClock_TickerDropsTicksItCannotDeliver(t *testing.T) {
	t.Parallel()

	clock := simenv.New(3).Clock()
	ticker := clock.NewTicker(time.Second)
	defer ticker.Stop()

	clock.Advance(5 * time.Second)

	select {
	case <-ticker.C():
	default:
		t.Fatal("no tick after five periods")
	}
	select {
	case got := <-ticker.C():
		t.Errorf("second tick at %v, want the buffered tick to be the only one", got)
	default:
	}
}

func TestClock_StoppedTickerNoLongerFires(t *testing.T) {
	t.Parallel()

	clock := simenv.New(4).Clock()
	ticker := clock.NewTicker(time.Second)
	ticker.Stop()

	clock.Advance(3 * time.Second)

	select {
	case got := <-ticker.C():
		t.Errorf("stopped ticker fired at %v", got)
	default:
	}
}

func TestClock_NewTickerPanicsOnNonPositivePeriod(t *testing.T) {
	t.Parallel()

	clock := simenv.New(5).Clock()
	wantPanic(t, "zero ticker period", func() {
		clock.NewTicker(0)
	})
	wantPanic(t, "negative ticker period", func() {
		clock.NewTicker(-time.Second)
	})
}
