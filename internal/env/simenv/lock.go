package simenv

import "io"

type lockHandle struct {
	fs *FS
}

var _ io.Closer = (*lockHandle)(nil)

func (l *lockHandle) Close() error {
	l.fs.unlock()
	return nil
}
