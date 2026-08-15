package keys

type Kind uint8

const (
	KindSet    Kind = 1
	KindDelete Kind = 2
)

type Seq uint64

const (
	MaxSeq      Seq = 1<<56 - 1
	TrailerSize     = 8
)

func (k Kind) Valid() bool {
	return k == KindSet || k == KindDelete
}
