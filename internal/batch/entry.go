package batch

import "github.com/devosher01/cairn/internal/keys"

type Entry struct {
	Seq   keys.Seq
	Kind  keys.Kind
	Key   []byte
	Value []byte
}
