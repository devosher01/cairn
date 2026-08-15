package manifest

import (
	"encoding/binary"
	"hash/crc32"
)

func Encode(s State) []byte {
	buf := make([]byte, 0, encodedSize(s))
	buf = append(buf, _magic...)
	buf = binary.LittleEndian.AppendUint32(buf, _version)
	buf = binary.LittleEndian.AppendUint64(buf, s.NextFileNum)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(s.LastSeq))
	buf = binary.LittleEndian.AppendUint64(buf, s.OldestWAL)
	buf = append(buf, NumLevels)

	for _, level := range s.Levels {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(level)))
		for _, t := range level {
			buf = appendTable(buf, t)
		}
	}

	return binary.LittleEndian.AppendUint32(buf, crc32.Checksum(buf, _crcTable))
}

func appendTable(dst []byte, t Table) []byte {
	dst = binary.LittleEndian.AppendUint64(dst, t.FileNum)
	dst = binary.LittleEndian.AppendUint64(dst, t.Size)
	dst = binary.LittleEndian.AppendUint64(dst, t.EntryCount)
	dst = binary.AppendUvarint(dst, uint64(len(t.Smallest)))
	dst = append(dst, t.Smallest...)
	dst = binary.AppendUvarint(dst, uint64(len(t.Largest)))

	return append(dst, t.Largest...)
}

func encodedSize(s State) int {
	n := _prefixSize + NumLevels*_u32Size + _crcSize
	for _, level := range s.Levels {
		for _, t := range level {
			n += _tableFixedSize +
				uvarintSize(len(t.Smallest)) + len(t.Smallest) +
				uvarintSize(len(t.Largest)) + len(t.Largest)
		}
	}

	return n
}

func uvarintSize(n int) int {
	size := 1
	for v := uint64(n); v >= 0x80; v >>= 7 {
		size++
	}

	return size
}
