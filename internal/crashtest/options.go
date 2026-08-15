package crashtest

const (
	_defaultTornByteLimit  = 4096
	_defaultTornStride     = 512
	_defaultScatterSamples = 2
)

type Options struct {
	TornByteLimit  int
	TornStride     int
	ScatterSamples int
	ScatterSeed    uint64
}

func (o Options) withDefaults() Options {
	if o.TornByteLimit <= 0 {
		o.TornByteLimit = _defaultTornByteLimit
	}
	if o.TornStride <= 0 {
		o.TornStride = _defaultTornStride
	}
	if o.ScatterSamples == 0 {
		o.ScatterSamples = _defaultScatterSamples
	}

	return o
}
