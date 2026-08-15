package manifest_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/manifest"
)

func TestLoad_ReportsAMissingManifest(t *testing.T) {
	t.Parallel()

	got, exists, err := manifest.Load(simenv.New(0).Env().FS)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if exists {
		t.Error("Load reported an existing manifest on an empty directory")
	}
	if !reflect.DeepEqual(got, manifest.State{}) {
		t.Errorf("Load returned state %+v, want the zero state", got)
	}
}

func TestLoad_ReportsACorruptManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give []byte
	}{
		{name: "empty file", give: nil},
		{name: "garbage", give: []byte("not a manifest at all, not even close to one")},
		{name: "flipped byte", give: patch(goldenBytes(t), 20, 0xff)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(0)
			writeFile(t, sim.Env().FS, manifest.FileName, tt.give)

			got, exists, err := manifest.Load(sim.Env().FS)
			if !errors.Is(err, manifest.ErrCorrupt) {
				t.Fatalf("Load returned error %v, want one wrapping ErrCorrupt", err)
			}
			if !exists {
				t.Error("Load reported no manifest for a file that exists")
			}
			if !reflect.DeepEqual(got, manifest.State{}) {
				t.Errorf("Load returned state %+v, want the zero state", got)
			}
		})
	}
}

func TestLoad_PropagatesFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		giveFS     fakeFS
		wantExists bool
		wantErr    error
	}{
		{
			name:    "open fails",
			giveFS:  fakeFS{openErr: errBoom},
			wantErr: errBoom,
		},
		{
			name:       "size fails",
			giveFS:     fakeFS{file: &fakeFile{sizeErr: errBoom}},
			wantExists: true,
			wantErr:    errBoom,
		},
		{
			name:       "file too large to be a manifest",
			giveFS:     fakeFS{file: &fakeFile{size: 1 << 40}},
			wantExists: true,
			wantErr:    manifest.ErrCorrupt,
		},
		{
			name:       "read fails",
			giveFS:     fakeFS{file: &fakeFile{size: 128, readErr: errBoom}},
			wantExists: true,
			wantErr:    errBoom,
		},
		{
			name:       "file shorter than its reported size",
			giveFS:     fakeFS{file: &fakeFile{size: 128}},
			wantExists: true,
			wantErr:    manifest.ErrCorrupt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, exists, err := manifest.Load(&tt.giveFS)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Load returned error %v, want one wrapping %v", err, tt.wantErr)
			}
			if exists != tt.wantExists {
				t.Errorf("Load reported exists = %t, want %t", exists, tt.wantExists)
			}
			if !reflect.DeepEqual(got, manifest.State{}) {
				t.Errorf("Load returned state %+v, want the zero state", got)
			}
		})
	}
}
