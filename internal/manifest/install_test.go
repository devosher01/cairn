package manifest_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/devosher01/cairn/internal/env/simenv"
	"github.com/devosher01/cairn/internal/manifest"
)

func TestInstallLoad_RoundTripsState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give manifest.State
	}{
		{name: "empty state", give: manifest.State{}},
		{name: "tables on several levels", give: goldenState()},
		{name: "state installed over another", give: campaignStateB()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(0)
			installOn(t, sim, campaignStateA(), tt.give)

			got, exists, err := manifest.Load(sim.Env().FS)
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if !exists {
				t.Fatal("Load reported no manifest after Install")
			}
			if !reflect.DeepEqual(got, tt.give) {
				t.Errorf("Load returned %+v, want %+v", got, tt.give)
			}
		})
	}
}

func TestInstall_LeavesTheDirectoryHoldingOnlyTheManifest(t *testing.T) {
	t.Parallel()

	sim := simenv.New(0)
	installOn(t, sim, goldenState())

	names, err := sim.Env().FS.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if want := []string{manifest.FileName}; !slices.Equal(names, want) {
		t.Errorf("directory holds %v, want %v", names, want)
	}
}

func TestInstall_PropagatesFilesystemErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		giveOp    int
		wantFiles []string
	}{
		{name: "create fails", giveOp: 0},
		{name: "write fails", giveOp: 1, wantFiles: []string{"MANIFEST.tmp"}},
		{name: "sync fails", giveOp: 2, wantFiles: []string{"MANIFEST.tmp"}},
		{name: "rename fails", giveOp: 3, wantFiles: []string{"MANIFEST.tmp"}},
		{name: "directory sync fails", giveOp: 4, wantFiles: []string{manifest.FileName}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sim := simenv.New(0)
			sim.InjectFault(tt.giveOp, errBoom)

			err := manifest.Install(sim.Env().FS, goldenState())
			if !errors.Is(err, errBoom) {
				t.Fatalf("Install returned error %v, want one wrapping errBoom", err)
			}

			names, err := sim.Env().FS.List()
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if !slices.Equal(names, tt.wantFiles) {
				t.Errorf("directory holds %v, want %v", names, tt.wantFiles)
			}
		})
	}
}

func TestInstall_PropagatesCloseErrors(t *testing.T) {
	t.Parallel()

	dir := &fakeFS{file: &fakeFile{closeErr: errBoom}}
	if err := manifest.Install(dir, goldenState()); !errors.Is(err, errBoom) {
		t.Fatalf("Install returned error %v, want one wrapping errBoom", err)
	}
}

func TestInstall_RejectsAShortWrite(t *testing.T) {
	t.Parallel()

	dir := &fakeFS{file: &fakeFile{shortWrite: 8}}
	if err := manifest.Install(dir, goldenState()); err == nil {
		t.Fatal("Install returned no error after a short write")
	}
}
