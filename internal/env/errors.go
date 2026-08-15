package env

import "errors"

var (
	ErrLocked  = errors.New("env: locked")
	ErrNoSpace = errors.New("env: no space")
)
