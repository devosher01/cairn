package memtable

import "sync/atomic"

const _nodeOverhead = 48

type node struct {
	buf    []byte
	keyLen int
	next   []atomic.Pointer[node]
}

func newNode(ikey, value []byte, height int) *node {
	buf := make([]byte, 0, len(ikey)+len(value))
	buf = append(buf, ikey...)
	buf = append(buf, value...)

	return &node{buf: buf, keyLen: len(ikey), next: make([]atomic.Pointer[node], height)}
}

func newHead() *node {
	return &node{next: make([]atomic.Pointer[node], _maxHeight)}
}

func (n *node) ikey() []byte {
	return n.buf[:n.keyLen]
}

func (n *node) value() []byte {
	return n.buf[n.keyLen:]
}

func (n *node) retained() int64 {
	return int64(len(n.buf)) + _nodeOverhead
}
