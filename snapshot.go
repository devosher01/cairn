package cairn

import "github.com/devosher01/cairn/internal/engine"

type Snapshot struct {
	s *engine.Snapshot
}

func (s *Snapshot) Get(key []byte) ([]byte, error) {
	return s.s.Get(key)
}

func (s *Snapshot) NewIterator(opts IterOptions) (*Iterator, error) {
	it, err := s.s.NewIterator(engine.IterOptions(opts))
	if err != nil {
		return nil, err
	}
	return &Iterator{it: it}, nil
}

func (s *Snapshot) Close() error {
	return s.s.Close()
}
