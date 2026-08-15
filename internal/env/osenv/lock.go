package osenv

import (
	"errors"
	"io"
	"os"
	"syscall"
)

type fileLock struct {
	f *os.File
}

var _ io.Closer = (*fileLock)(nil)

func (l *fileLock) Close() error {
	unlockErr := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return errors.Join(unlockErr, l.f.Close())
}
