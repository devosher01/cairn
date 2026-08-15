package keys

import "encoding/binary"

const _seekKind = 0xFF

func Append(dst, user []byte, seq Seq, kind Kind) []byte {
	dst = append(dst, user...)
	return binary.LittleEndian.AppendUint64(dst, uint64(seq)<<8|uint64(kind))
}

func AppendSeek(dst, user []byte, seq Seq) []byte {
	dst = append(dst, user...)
	return binary.LittleEndian.AppendUint64(dst, uint64(seq)<<8|_seekKind)
}

func UserKey(ikey []byte) []byte {
	return ikey[:len(ikey)-TrailerSize]
}

func Trailer(ikey []byte) (Seq, Kind) {
	t := binary.LittleEndian.Uint64(ikey[len(ikey)-TrailerSize:])
	return Seq(t >> 8), Kind(t)
}
