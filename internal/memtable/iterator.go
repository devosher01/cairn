package memtable

type Iterator struct {
	m   *Memtable
	cur *node
}

func (m *Memtable) Iter() *Iterator {
	return &Iterator{m: m}
}

func (i *Iterator) SeekGE(ikey []byte) {
	i.cur = i.m.findGE(ikey, nil)
}

func (i *Iterator) First() {
	i.cur = i.m.head.next[0].Load()
}

func (i *Iterator) Next() {
	i.cur = i.positioned().next[0].Load()
}

func (i *Iterator) Valid() bool {
	return i.cur != nil
}

func (i *Iterator) Key() []byte {
	return i.positioned().ikey()
}

func (i *Iterator) Value() []byte {
	return i.positioned().value()
}

func (i *Iterator) positioned() *node {
	if i.cur == nil {
		panic("memtable: iterator is not positioned on an entry")
	}

	return i.cur
}
