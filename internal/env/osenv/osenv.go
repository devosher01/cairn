package osenv

import (
	"os"

	"github.com/devosher01/cairn/internal/env"
)

const (
	_dirPerm  = 0o755
	_filePerm = 0o644
)

func New(dir string) (env.Env, error) {
	if err := os.MkdirAll(dir, _dirPerm); err != nil {
		return env.Env{}, err
	}
	return env.Env{
		FS:    &fileSystem{dir: dir},
		Clock: clock{},
		Rand:  rng{},
	}, nil
}
