package memtable

import "github.com/devosher01/cairn/internal/keys"

const (
	_maxHeight = 12
	_branching = 4
)

func (m *Memtable) findGE(ikey []byte, prev *[_maxHeight]*node) *node {
	current := m.head
	for level := int(m.height.Load()) - 1; level >= 0; level-- {
		next := current.next[level].Load()
		for next != nil && keys.Compare(next.ikey(), ikey) < 0 {
			current = next
			next = current.next[level].Load()
		}
		if prev != nil {
			prev[level] = current
		}
	}

	return current.next[0].Load()
}

func (m *Memtable) link(n *node, prev *[_maxHeight]*node) {
	height := len(n.next)
	if height > int(m.height.Load()) {
		m.height.Store(int32(height))
	}

	for level := range height {
		n.next[level].Store(prev[level].next[level].Load())
	}
	for level := range height {
		prev[level].next[level].Store(n)
	}
}

func (m *Memtable) randomHeight() int {
	height := 1
	for height < _maxHeight && m.rnd.Uint64()%_branching == 0 {
		height++
	}

	return height
}
