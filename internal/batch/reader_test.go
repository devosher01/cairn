package batch_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/devosher01/cairn/internal/batch"
	"github.com/devosher01/cairn/internal/keys"
)

func TestNewReader_RejectsShortHeader(t *testing.T) {
	t.Parallel()

	if _, err := batch.NewReader(make([]byte, 11)); !errors.Is(err, batch.ErrCorrupt) {
		t.Errorf("NewReader error = %v, want ErrCorrupt", err)
	}
}

func TestReader_SurfacesCorruption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []byte
	}{
		{
			name: "header shorter than the fixed size",
			give: make([]byte, 11),
		},
		{
			name: "empty payload",
			give: nil,
		},
		{
			name: "count larger than the entries present",
			give: header(1, 2, 0x01, 0x01, 'k', 0x01, 'v'),
		},
		{
			name: "kind zero",
			give: header(1, 1, 0x00, 0x01, 'k'),
		},
		{
			name: "kind three",
			give: header(1, 1, 0x03, 0x01, 'k'),
		},
		{
			name: "key length prefix truncated",
			give: header(1, 1, 0x01, 0x80),
		},
		{
			name: "key length overruns the payload",
			give: header(1, 1, 0x01, 0x64, 'k'),
		},
		{
			name: "value length overruns the payload",
			give: header(1, 1, 0x01, 0x01, 'k', 0x64, 'v'),
		},
		{
			name: "key length above the uint32 maximum",
			give: append(header(1, 1, 0x01), binary.AppendUvarint(nil, 1<<40)...),
		},
		{
			name: "trailing garbage after the last entry",
			give: append(build(3, []entry{{kind: keys.KindDelete, key: []byte("k")}}), 0xff),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := batch.NewReader(tt.give)
			if err != nil {
				if !errors.Is(err, batch.ErrCorrupt) {
					t.Errorf("NewReader error = %v, want ErrCorrupt", err)
				}

				return
			}

			for {
				if _, ok := r.Next(); !ok {
					break
				}
			}
			if !errors.Is(r.Err(), batch.ErrCorrupt) {
				t.Errorf("Err after the walk = %v, want ErrCorrupt", r.Err())
			}
		})
	}
}
