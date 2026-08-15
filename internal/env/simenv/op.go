package simenv

type OpKind uint8

const (
	OpCreate OpKind = iota + 1
	OpWrite
	OpSync
	OpRemove
	OpRename
	OpSyncDir
)

type Op struct {
	Kind   OpKind
	Name   string
	To     string
	Off    int64
	Len    int
	Failed bool
}

type opRec struct {
	fileID int
	data   []byte
}
