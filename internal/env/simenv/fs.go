package simenv

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"sync"

	"github.com/devosher01/cairn/internal/env"
)

type FS struct {
	mu     *sync.Mutex
	dir    map[string]*file
	ops    []Op
	recs   []opRec
	faults map[int]error
	budget int64
	locked bool
	nextID int
}

var _ env.FS = (*FS)(nil)

func newFS(mu *sync.Mutex) *FS {
	return &FS{
		mu:     mu,
		dir:    make(map[string]*file),
		faults: make(map[int]error),
		budget: -1,
	}
}

func (f *FS) Create(name string) (env.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpCreate, Name: name}
	if err := f.injected(); err != nil {
		return nil, f.failOp(op, err)
	}
	f.nextID++
	fl := &file{id: f.nextID, name: name}
	f.dir[name] = fl
	f.appendOp(op, opRec{fileID: fl.id})
	return &writeHandle{fs: f, file: fl}, nil
}

func (f *FS) Open(name string) (env.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fl, ok := f.dir[name]
	if !ok {
		return nil, fmt.Errorf("simenv: open %s: %w", name, fs.ErrNotExist)
	}
	return &readHandle{fs: f, file: fl}, nil
}

func (f *FS) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpRemove, Name: name}
	if err := f.injected(); err != nil {
		return f.failOp(op, err)
	}
	delete(f.dir, name)
	f.appendOp(op, opRec{})
	return nil
}

func (f *FS) Rename(oldname, newname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpRename, Name: oldname, To: newname}
	if err := f.injected(); err != nil {
		return f.failOp(op, err)
	}
	fl, ok := f.dir[oldname]
	if !ok {
		return f.failOp(op, fmt.Errorf("simenv: rename %s: %w", oldname, fs.ErrNotExist))
	}
	delete(f.dir, oldname)
	f.dir[newname] = fl
	f.appendOp(op, opRec{})
	return nil
}

func (f *FS) List() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Sorted(maps.Keys(f.dir)), nil
}

func (f *FS) SyncDir() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpSyncDir}
	if err := f.injected(); err != nil {
		return f.failOp(op, err)
	}
	f.appendOp(op, opRec{})
	return nil
}

func (f *FS) Lock() (io.Closer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.locked {
		return nil, env.ErrLocked
	}
	f.locked = true
	return &lockHandle{fs: f}, nil
}

func (f *FS) write(h *writeHandle, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpWrite, Name: h.file.name, Off: h.off}
	if err := f.injected(); err != nil {
		return 0, f.failOp(op, err)
	}

	n := len(p)
	short := f.budget >= 0 && int64(n) > f.budget
	if short {
		n = int(f.budget)
	}
	if f.budget >= 0 {
		f.budget -= int64(n)
	}

	data := bytes.Clone(p[:n])
	h.file.write(h.off, data)
	h.off += int64(n)

	op.Len = n
	op.Failed = short
	f.appendOp(op, opRec{fileID: h.file.id, data: data})
	if short {
		return n, env.ErrNoSpace
	}
	return n, nil
}

func (f *FS) syncFile(fl *file) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	op := Op{Kind: OpSync, Name: fl.name}
	if err := f.injected(); err != nil {
		return f.failOp(op, err)
	}
	f.appendOp(op, opRec{fileID: fl.id})
	return nil
}

func (f *FS) unlock() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.locked = false
}

func (f *FS) opLog() []Op {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.ops)
}

func (f *FS) injectFault(atOp int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.faults[atOp] = err
}

func (f *FS) setDiskBudget(n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.budget = n
}

func (f *FS) injected() error {
	return f.faults[len(f.ops)]
}

func (f *FS) appendOp(op Op, rec opRec) {
	f.ops = append(f.ops, op)
	f.recs = append(f.recs, rec)
}

func (f *FS) failOp(op Op, err error) error {
	op.Failed = true
	f.appendOp(op, opRec{})
	return err
}
