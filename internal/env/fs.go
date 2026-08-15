package env

import "io"

type FS interface {
	Create(name string) (File, error)
	Open(name string) (File, error)
	Remove(name string) error
	Rename(oldname, newname string) error
	List() ([]string, error)
	SyncDir() error
	Lock() (io.Closer, error)
}

type File interface {
	io.ReaderAt
	io.Writer
	Sync() error
	Close() error
	Size() (int64, error)
}
