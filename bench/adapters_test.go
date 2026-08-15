package bench

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble/v2"
	"go.etcd.io/bbolt"

	"github.com/devosher01/cairn"
)

var errMissing = errors.New("bench: key not found")

type store interface {
	open(tb testing.TB, dir string, sync bool)
	put(key, value []byte) error
	get(key []byte) ([]byte, error)
	scan(fn func(key, value []byte)) error
	close() error
}

type cairnStore struct {
	db *cairn.DB
}

func (s *cairnStore) open(tb testing.TB, dir string, sync bool) {
	mode := cairn.SyncOff
	if sync {
		mode = cairn.SyncAlways
	}
	db, err := cairn.Open(dir, &cairn.Options{Sync: mode})
	if err != nil {
		tb.Fatalf("cairn open: %v", err)
	}
	s.db = db
}

func (s *cairnStore) put(key, value []byte) error {
	return s.db.Put(key, value)
}

func (s *cairnStore) get(key []byte) ([]byte, error) {
	return s.db.Get(key)
}

func (s *cairnStore) scan(fn func(key, value []byte)) error {
	it, err := s.db.NewIterator(cairn.IterOptions{})
	if err != nil {
		return err
	}
	for ok := it.First(); ok; ok = it.Next() {
		fn(it.Key(), it.Value())
	}
	if err := it.Error(); err != nil {
		_ = it.Close()
		return err
	}
	return it.Close()
}

func (s *cairnStore) close() error {
	return s.db.Close()
}

const (
	_boltFile = "bench.db"
	_boltMode = 0o600
)

var _bucketName = []byte("bench")

type boltStore struct {
	db *bbolt.DB
}

func (s *boltStore) open(tb testing.TB, dir string, sync bool) {
	db, err := bbolt.Open(filepath.Join(dir, _boltFile), _boltMode, &bbolt.Options{NoSync: !sync})
	if err != nil {
		tb.Fatalf("bolt open: %v", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		_, createErr := tx.CreateBucketIfNotExists(_bucketName)
		return createErr
	})
	if err != nil {
		tb.Fatalf("bolt bucket: %v", err)
	}
	s.db = db
}

func (s *boltStore) put(key, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(_bucketName).Put(key, value)
	})
}

func (s *boltStore) get(key []byte) ([]byte, error) {
	var out []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		value := tx.Bucket(_bucketName).Get(key)
		if value == nil {
			return errMissing
		}
		out = bytes.Clone(value)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *boltStore) scan(fn func(key, value []byte)) error {
	return s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(_bucketName).ForEach(func(key, value []byte) error {
			fn(key, value)
			return nil
		})
	})
}

func (s *boltStore) close() error {
	return s.db.Close()
}

type quietLogger struct{}

func (quietLogger) Infof(string, ...any) {}

func (quietLogger) Errorf(format string, args ...any) {
	pebble.DefaultLogger.Errorf(format, args...)
}

func (quietLogger) Fatalf(format string, args ...any) {
	pebble.DefaultLogger.Fatalf(format, args...)
}

type pebbleStore struct {
	db    *pebble.DB
	write *pebble.WriteOptions
}

func (s *pebbleStore) open(tb testing.TB, dir string, sync bool) {
	db, err := pebble.Open(dir, &pebble.Options{Logger: quietLogger{}})
	if err != nil {
		tb.Fatalf("pebble open: %v", err)
	}
	s.db = db
	s.write = &pebble.WriteOptions{Sync: sync}
}

func (s *pebbleStore) put(key, value []byte) error {
	return s.db.Set(key, value, s.write)
}

func (s *pebbleStore) get(key []byte) ([]byte, error) {
	value, closer, err := s.db.Get(key)
	if err != nil {
		return nil, err
	}
	out := bytes.Clone(value)
	if err := closer.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pebbleStore) scan(fn func(key, value []byte)) error {
	it, err := s.db.NewIter(nil)
	if err != nil {
		return err
	}
	for ok := it.First(); ok; ok = it.Next() {
		fn(it.Key(), it.Value())
	}
	return it.Close()
}

func (s *pebbleStore) close() error {
	return s.db.Close()
}
