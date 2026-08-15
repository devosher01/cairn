package engine

const (
	MaxKeySize   = 64 << 10
	MaxValueSize = 4 << 20
)

func validateKey(key []byte) error {
	if len(key) == 0 || len(key) > MaxKeySize {
		return ErrInvalidKey
	}
	return nil
}

func validateValue(value []byte) error {
	if len(value) > MaxValueSize {
		return ErrInvalidValue
	}
	return nil
}
