package simenv

type file struct {
	id   int
	name string
	data []byte
}

func (f *file) write(off int64, data []byte) {
	f.data = writeInto(f.data, off, data)
}

func writeInto(dst []byte, off int64, data []byte) []byte {
	end := off + int64(len(data))
	if end > int64(len(dst)) {
		grown := make([]byte, end)
		copy(grown, dst)
		dst = grown
	}
	copy(dst[off:end], data)
	return dst
}
