package manifest

import (
	"hash/crc32"

	"github.com/devosher01/cairn/internal/keys"
)

const (
	FileName     = "MANIFEST"
	_tmpFileName = "MANIFEST.tmp"
)

const (
	_magic   = "CAIRNMAN"
	_version = 1
)

const (
	_magicSize      = 8
	_versionSize    = 4
	_headerSize     = _magicSize + _versionSize
	_u32Size        = 4
	_u64Size        = 8
	_levelCountSize = 1
	_crcSize        = 4

	_prefixSize     = _headerSize + 3*_u64Size + _levelCountSize
	_tableFixedSize = 3 * _u64Size
	_minKeySize     = keys.TrailerSize + 1
	_minTableSize   = _tableFixedSize + 2*(1+_minKeySize)
	_minSize        = _prefixSize + NumLevels*_u32Size + _crcSize
	_maxFileSize    = 256 << 20
)

var _crcTable = crc32.MakeTable(crc32.Castagnoli)
