package sstable

import "slices"

type blockEntry struct {
	ikey  string
	value string
}

func buildBlockPayload(entries []blockEntry) []byte {
	b := newBlockBuilder()
	for _, e := range entries {
		b.add([]byte(e.ikey), []byte(e.value))
	}

	return b.finish()
}

func collectBlockEntries(payload []byte) ([]blockEntry, error) {
	it := newBlockIter(payload)

	var got []blockEntry
	for {
		ikey, value, ok := it.next()
		if !ok {
			break
		}
		got = append(got, blockEntry{ikey: string(ikey), value: string(value)})
	}

	return got, it.err()
}

func flipBlockByte(raw []byte, at int) []byte {
	out := slices.Clone(raw)
	out[at] ^= 0xFF

	return out
}
