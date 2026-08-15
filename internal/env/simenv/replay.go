package simenv

import (
	"bytes"
	"maps"
)

const _sectorSize = 512

type image struct {
	name     string
	durable  []byte
	volatile []byte
	tail     []tailWrite
}

type tailWrite struct {
	off  int64
	data []byte
}

type dirOp struct {
	index int
	kind  OpKind
	name  string
	to    string
	id    int
}

type replay struct {
	images  map[int]*image
	dirDur  map[string]int
	dirVol  map[string]int
	dirTail []dirOp
}

func newReplay() *replay {
	return &replay{
		images: make(map[int]*image),
		dirDur: make(map[string]int),
		dirVol: make(map[string]int),
	}
}

func (r *replay) apply(index int, op Op, rec opRec) {
	if op.Failed {
		if op.Kind == OpWrite && len(rec.data) > 0 {
			r.imageOf(rec.fileID).write(op.Off, rec.data)
		}
		return
	}
	switch op.Kind {
	case OpCreate:
		r.images[rec.fileID] = &image{name: op.Name}
		r.dirVol[op.Name] = rec.fileID
		r.dirTail = append(r.dirTail, dirOp{index: index, kind: OpCreate, name: op.Name, id: rec.fileID})
	case OpWrite:
		r.imageOf(rec.fileID).write(op.Off, rec.data)
	case OpSync:
		im := r.imageOf(rec.fileID)
		im.durable = bytes.Clone(im.volatile)
		im.tail = nil
	case OpRemove:
		delete(r.dirVol, op.Name)
		r.dirTail = append(r.dirTail, dirOp{index: index, kind: OpRemove, name: op.Name})
	case OpRename:
		id, ok := r.dirVol[op.Name]
		if !ok {
			panic("simenv: replayed rename without a source")
		}
		delete(r.dirVol, op.Name)
		r.dirVol[op.To] = id
		r.dirTail = append(r.dirTail, dirOp{index: index, kind: OpRename, name: op.Name, to: op.To})
	case OpSyncDir:
		r.dirDur = maps.Clone(r.dirVol)
		r.dirTail = nil
	default:
		panic("simenv: replayed op of unknown kind")
	}
}

func (r *replay) imageOf(id int) *image {
	im, ok := r.images[id]
	if !ok {
		panic("simenv: replayed op on an unknown file")
	}
	return im
}

func (r *replay) contents(mode CrashMode, seed uint64) map[int][]byte {
	out := make(map[int][]byte, len(r.images))
	for id, im := range r.images {
		switch mode {
		case CrashNone:
			out[id] = bytes.Clone(im.durable)
		case CrashPrefix:
			out[id] = bytes.Clone(im.volatile)
		case CrashScatter:
			out[id] = im.scatter(seed)
		default:
			panic("simenv: unknown crash mode")
		}
	}
	return out
}

func (r *replay) dir(mode CrashMode, seed uint64) map[string]int {
	switch mode {
	case CrashNone:
		return maps.Clone(r.dirDur)
	case CrashPrefix:
		return maps.Clone(r.dirVol)
	case CrashScatter:
		return r.scatterDir(seed)
	default:
		panic("simenv: unknown crash mode")
	}
}

func (r *replay) scatterDir(seed uint64) map[string]int {
	out := maps.Clone(r.dirDur)
	for _, d := range r.dirTail {
		if !coinDir(seed, d.index) {
			continue
		}
		switch d.kind {
		case OpCreate:
			out[d.name] = d.id
		case OpRemove:
			delete(out, d.name)
		case OpRename:
			id, ok := out[d.name]
			if !ok {
				continue
			}
			delete(out, d.name)
			out[d.to] = id
		}
	}
	return out
}

func (im *image) write(off int64, data []byte) {
	im.volatile = writeInto(im.volatile, off, data)
	im.tail = append(im.tail, tailWrite{off: off, data: data})
}

func (im *image) scatter(seed uint64) []byte {
	survivors := make(map[int]bool)
	for _, w := range im.tail {
		if len(w.data) == 0 {
			continue
		}
		first := int(w.off / _sectorSize)
		last := int((w.off + int64(len(w.data)) - 1) / _sectorSize)
		for s := first; s <= last; s++ {
			if _, seen := survivors[s]; seen {
				continue
			}
			survivors[s] = coinSector(seed, im.name, s)
		}
	}

	size := len(im.durable)
	for s, alive := range survivors {
		if !alive {
			continue
		}
		size = max(size, min((s+1)*_sectorSize, len(im.volatile)))
	}

	out := make([]byte, size)
	copy(out, im.durable)
	for s, alive := range survivors {
		if !alive {
			continue
		}
		lo := s * _sectorSize
		hi := min(lo+_sectorSize, len(im.volatile))
		copy(out[lo:hi], im.volatile[lo:hi])
	}
	return out
}
