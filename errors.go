package cairn

import "github.com/devosher01/cairn/internal/engine"

var (
	ErrNotFound      = engine.ErrNotFound
	ErrClosed        = engine.ErrClosed
	ErrLocked        = engine.ErrLocked
	ErrCorruption    = engine.ErrCorruption
	ErrDBFailed      = engine.ErrDBFailed
	ErrInvalidKey    = engine.ErrInvalidKey
	ErrInvalidValue  = engine.ErrInvalidValue
	ErrBatchTooLarge = engine.ErrBatchTooLarge
	ErrOpenHandles   = engine.ErrOpenHandles
)
