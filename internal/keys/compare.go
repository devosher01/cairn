package keys

import (
	"bytes"
	"encoding/binary"
)

func Compare(a, b []byte) int {
	if c := bytes.Compare(UserKey(a), UserKey(b)); c != 0 {
		return c
	}
	ta := binary.LittleEndian.Uint64(a[len(a)-TrailerSize:])
	tb := binary.LittleEndian.Uint64(b[len(b)-TrailerSize:])
	switch {
	case ta > tb:
		return -1
	case ta < tb:
		return 1
	default:
		return 0
	}
}
