package cairn

import (
	"github.com/devosher01/cairn/internal/env"
	"github.com/devosher01/cairn/internal/env/osenv"
)

type SyncMode uint8

const (
	SyncAlways SyncMode = iota
	SyncOff
)

type Options struct {
	Env  env.Env
	Sync SyncMode
}

func (o *Options) resolved(dir string) (Options, error) {
	var out Options
	if o != nil {
		out = *o
	}
	if out.Env.FS == nil {
		real, err := osenv.New(dir)
		if err != nil {
			return Options{}, err
		}
		out.Env = real
	}
	return out, nil
}
