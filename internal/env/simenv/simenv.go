package simenv

import (
	"sync"

	"github.com/devosher01/cairn/internal/env"
)

type Sim struct {
	fs    *FS
	clock *Clock
	rand  *simRand
}

func New(seed uint64) *Sim {
	mu := new(sync.Mutex)
	return &Sim{
		fs:    newFS(mu),
		clock: newClock(mu),
		rand:  newRand(mu, seed),
	}
}

func (s *Sim) Env() env.Env {
	return env.Env{FS: s.fs, Clock: s.clock, Rand: s.rand}
}

func (s *Sim) FS() *FS {
	return s.fs
}

func (s *Sim) Clock() *Clock {
	return s.clock
}

func (s *Sim) Ops() []Op {
	return s.fs.opLog()
}

func (s *Sim) MaterializeCrash(p CrashPoint) *FS {
	return s.fs.materialize(p)
}

func (s *Sim) InjectFault(atOp int, err error) {
	s.fs.injectFault(atOp, err)
}

func (s *Sim) SetDiskBudget(n int64) {
	s.fs.setDiskBudget(n)
}
