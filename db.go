package cairn

import "github.com/devosher01/cairn/internal/engine"

type DB struct {
	e *engine.DB
}

func Open(dir string, opts *Options) (*DB, error) {
	e, err := engine.Open(dir, opts.engine())
	if err != nil {
		return nil, err
	}
	return &DB{e: e}, nil
}

func (db *DB) Get(key []byte) ([]byte, error) {
	return db.e.Get(key)
}

func (db *DB) Put(key, value []byte) error {
	return db.e.Put(key, value)
}

func (db *DB) Delete(key []byte) error {
	return db.e.Delete(key)
}

func (db *DB) Write(b *Batch) error {
	return db.e.Write(b.inner)
}

func (db *DB) Flush() error {
	return db.e.Flush()
}

func (db *DB) Compact() error {
	return db.e.Compact()
}

func (db *DB) NewSnapshot() (*Snapshot, error) {
	s, err := db.e.NewSnapshot()
	if err != nil {
		return nil, err
	}
	return &Snapshot{s: s}, nil
}

func (db *DB) NewIterator(opts IterOptions) (*Iterator, error) {
	it, err := db.e.NewIterator(engine.IterOptions(opts))
	if err != nil {
		return nil, err
	}
	return &Iterator{it: it}, nil
}

func (db *DB) Metrics() Metrics {
	return metricsFrom(db.e.Metrics())
}

func (db *DB) Close() error {
	return db.e.Close()
}
