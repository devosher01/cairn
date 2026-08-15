package simenv

import (
	"bytes"
	"sync"
)

type CrashMode uint8

const (
	CrashNone CrashMode = iota + 1
	CrashPrefix
	CrashScatter
)

type CrashPoint struct {
	Op          int
	Torn        int
	Mode        CrashMode
	ScatterSeed uint64
}

func (f *FS) materialize(p CrashPoint) *FS {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.checkCrashPoint(p)

	r := newReplay()
	for i := range p.Op {
		r.apply(i, f.ops[i], f.recs[i])
	}
	if p.Torn > 0 {
		torn := f.ops[p.Op]
		r.imageOf(f.recs[p.Op].fileID).write(torn.Off, f.recs[p.Op].data[:p.Torn])
	}

	contents := r.contents(p.Mode, p.ScatterSeed)
	out := newFS(new(sync.Mutex))
	for name, id := range r.dir(p.Mode, p.ScatterSeed) {
		out.dir[name] = &file{id: id, name: name, data: bytes.Clone(contents[id])}
		out.nextID = max(out.nextID, id)
	}
	return out
}

func (f *FS) checkCrashPoint(p CrashPoint) {
	switch p.Mode {
	case CrashNone, CrashPrefix, CrashScatter:
	default:
		panic("simenv: unknown crash mode")
	}
	if p.Op < 0 || p.Op > len(f.ops) {
		panic("simenv: crash point out of range")
	}
	if p.Torn == 0 {
		return
	}
	if p.Torn < 0 || p.Op == len(f.ops) {
		panic("simenv: torn crash point out of range")
	}
	op := f.ops[p.Op]
	if op.Kind != OpWrite || op.Failed || p.Torn >= op.Len {
		panic("simenv: torn crash point is not a partial write")
	}
}
