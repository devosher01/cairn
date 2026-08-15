package wal_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/wal"
)

func TestNewWriter_WritesHeaderInASingleWrite(t *testing.T) {
	t.Parallel()

	f := &fakeFile{}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	want := []byte("CAIRNWAL\x01\x00\x00\x00")
	if len(f.writes) != 1 {
		t.Fatalf("got %d writes, want 1", len(f.writes))
	}
	if !bytes.Equal(f.writes[0], want) {
		t.Errorf("got header %x, want %x", f.writes[0], want)
	}
	if w.Size() != int64(len(want)) {
		t.Errorf("got size %d, want %d", w.Size(), len(want))
	}
}

func TestNewWriter_PropagatesHeaderWriteError(t *testing.T) {
	t.Parallel()

	f := &fakeFile{writeErr: errBoom}
	w, err := wal.NewWriter(f)
	if !errors.Is(err, errBoom) {
		t.Fatalf("got error %v, want %v", err, errBoom)
	}
	if w != nil {
		t.Errorf("got writer %v, want nil", w)
	}
}

func TestWriter_AppendWritesOneFramePerRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		givePayloads [][]byte
		wantFrames   []int
		wantSize     int64
	}{
		{
			name:         "empty payload still frames a record",
			givePayloads: [][]byte{{}},
			wantFrames:   []int{8},
			wantSize:     20,
		},
		{
			name:         "single byte payload",
			givePayloads: [][]byte{{0x7f}},
			wantFrames:   []int{9},
			wantSize:     21,
		},
		{
			name:         "mixed sizes keep one write each",
			givePayloads: [][]byte{[]byte("alpha"), bytes.Repeat([]byte{0xab}, 5<<10), {}},
			wantFrames:   []int{13, 5128, 8},
			wantSize:     5161,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeFile{}
			w, err := wal.NewWriter(f)
			if err != nil {
				t.Fatalf("NewWriter returned error: %v", err)
			}
			for _, payload := range tt.givePayloads {
				if err := w.Append(payload); err != nil {
					t.Fatalf("Append returned error: %v", err)
				}
			}

			gotFrames := make([]int, 0, len(tt.wantFrames))
			for _, write := range f.writes[1:] {
				gotFrames = append(gotFrames, len(write))
			}
			if !slices.Equal(gotFrames, tt.wantFrames) {
				t.Errorf("got frame writes %v, want %v", gotFrames, tt.wantFrames)
			}
			if w.Size() != tt.wantSize {
				t.Errorf("got size %d, want %d", w.Size(), tt.wantSize)
			}
			if int64(len(f.data)) != tt.wantSize {
				t.Errorf("got %d bytes on disk, want %d", len(f.data), tt.wantSize)
			}
		})
	}
}

func TestWriter_AppendPropagatesWriteError(t *testing.T) {
	t.Parallel()

	f := &fakeFile{}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	f.writeErr = errBoom
	if err := w.Append([]byte("alpha")); !errors.Is(err, errBoom) {
		t.Fatalf("got error %v, want %v", err, errBoom)
	}
	if w.Size() != 12 {
		t.Errorf("got size %d, want 12", w.Size())
	}
}

func TestWriter_CloseSyncsThenClosesRegardlessOfSyncError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		giveSyncErr  error
		giveCloseErr error
		wantErr      error
	}{
		{
			name: "clean close",
		},
		{
			name:        "sync failure still closes the file",
			giveSyncErr: errBoom,
			wantErr:     errBoom,
		},
		{
			name:         "close failure surfaces when sync succeeds",
			giveCloseErr: errOther,
			wantErr:      errOther,
		},
		{
			name:         "sync failure wins over close failure",
			giveSyncErr:  errBoom,
			giveCloseErr: errOther,
			wantErr:      errBoom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeFile{syncErr: tt.giveSyncErr, closeErr: tt.giveCloseErr}
			w, err := wal.NewWriter(f)
			if err != nil {
				t.Fatalf("NewWriter returned error: %v", err)
			}

			err = w.Close()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("got error %v, want %v", err, tt.wantErr)
			}
			if f.syncs != 1 {
				t.Errorf("got %d syncs, want 1", f.syncs)
			}
			if f.closes != 1 {
				t.Errorf("got %d closes, want 1", f.closes)
			}
		})
	}
}

func TestWriter_SyncDelegatesToFile(t *testing.T) {
	t.Parallel()

	f := &fakeFile{}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	if err := w.Sync(); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	f.syncErr = errBoom
	if err := w.Sync(); !errors.Is(err, errBoom) {
		t.Fatalf("got error %v, want %v", err, errBoom)
	}
	if f.syncs != 2 {
		t.Errorf("got %d syncs, want 2", f.syncs)
	}
}
