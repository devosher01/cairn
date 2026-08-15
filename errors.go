package cairn

import "errors"

var (
	ErrNotFound     = errors.New("cairn: not found")
	ErrClosed       = errors.New("cairn: closed")
	ErrLocked       = errors.New("cairn: locked")
	ErrCorruption   = errors.New("cairn: corruption")
	ErrDBFailed     = errors.New("cairn: db failed")
	ErrInvalidKey   = errors.New("cairn: invalid key")
	ErrInvalidValue = errors.New("cairn: invalid value")
)
