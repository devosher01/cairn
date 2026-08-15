package wal_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/devosher01/cairn/internal/wal"
)

func TestReplay_RoundTripsAppendedPayloads(t *testing.T) {
	t.Parallel()

	want := [][]byte{
		{},
		{0x01},
		[]byte("alpha"),
		bytes.Repeat([]byte{0xab}, 5<<10),
		[]byte("omega"),
	}

	f := &fakeFile{}
	w, err := wal.NewWriter(f)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}
	for _, payload := range want {
		if err := w.Append(payload); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	got, offset, err := replayAll(f.data)
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if !equalPayloads(got, want) {
		t.Errorf("got %d payloads, want %d", len(got), len(want))
	}
	if offset != w.Size() {
		t.Errorf("got offset %d, want %d", offset, w.Size())
	}
}

func TestReplay_TreatsInvalidHeaderAsEmpty(t *testing.T) {
	t.Parallel()

	valid := buildWAL(t, []byte("alpha"))

	tests := []struct {
		name     string
		giveData []byte
	}{
		{
			name:     "empty file",
			giveData: nil,
		},
		{
			name:     "file shorter than the header",
			giveData: []byte("CAIRN"),
		},
		{
			name:     "wrong magic",
			giveData: flipByte(valid, 0),
		},
		{
			name:     "wrong version",
			giveData: flipByte(valid, 8),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, offset, err := replayAll(tt.giveData)
			if err != nil {
				t.Fatalf("Replay returned error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("got %d payloads, want 0", len(got))
			}
			if offset != 0 {
				t.Errorf("got offset %d, want 0", offset)
			}
		})
	}
}

func TestReplay_TruncatedTailKeepsValidPrefix(t *testing.T) {
	t.Parallel()

	want := [][]byte{[]byte("alpha"), []byte("bravo")}
	full := buildWAL(t, want[0], want[1], []byte("charlie"))
	lastRecord := len(buildWAL(t, want[0], want[1]))

	for cut := lastRecord; cut < len(full); cut++ {
		t.Run(fmt.Sprintf("truncated to %d bytes", cut), func(t *testing.T) {
			t.Parallel()

			got, offset, err := replayAll(full[:cut])
			if err != nil {
				t.Fatalf("Replay returned error: %v", err)
			}
			if !equalPayloads(got, want) {
				t.Errorf("got %d payloads, want %d", len(got), len(want))
			}
			if offset != int64(lastRecord) {
				t.Errorf("got offset %d, want %d", offset, lastRecord)
			}
		})
	}
}

func TestReplay_CorruptLastRecordIsDropped(t *testing.T) {
	t.Parallel()

	want := [][]byte{[]byte("alpha"), []byte("bravo")}
	full := buildWAL(t, want[0], want[1], []byte("charlie"))
	lastRecord := len(buildWAL(t, want[0], want[1]))

	tests := []struct {
		name             string
		giveFrameOffset  int
		wantRecordOffset int
	}{
		{
			name:             "corrupt crc",
			giveFrameOffset:  0,
			wantRecordOffset: lastRecord,
		},
		{
			name:             "corrupt length",
			giveFrameOffset:  4,
			wantRecordOffset: lastRecord,
		},
		{
			name:             "corrupt payload",
			giveFrameOffset:  8,
			wantRecordOffset: lastRecord,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, offset, err := replayAll(flipByte(full, lastRecord+tt.giveFrameOffset))
			if err != nil {
				t.Fatalf("Replay returned error: %v", err)
			}
			if !equalPayloads(got, want) {
				t.Errorf("got %d payloads, want %d", len(got), len(want))
			}
			if offset != int64(tt.wantRecordOffset) {
				t.Errorf("got offset %d, want %d", offset, tt.wantRecordOffset)
			}
		})
	}
}

func TestReplay_StopsAtCorruptMiddleRecord(t *testing.T) {
	t.Parallel()

	full := buildWAL(t,
		[]byte("alpha"),
		[]byte("bravo"),
		[]byte("charlie"),
		[]byte("delta"),
	)
	secondRecord := len(buildWAL(t, []byte("alpha")))
	want := [][]byte{[]byte("alpha")}

	got, offset, err := replayAll(flipByte(full, secondRecord+8))
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if !equalPayloads(got, want) {
		t.Errorf("got %d payloads, want %d", len(got), len(want))
	}
	if offset != int64(secondRecord) {
		t.Errorf("got offset %d, want %d", offset, secondRecord)
	}
}

func TestReplay_StopsAtLengthBeyondMaxPayload(t *testing.T) {
	t.Parallel()

	want := [][]byte{[]byte("alpha")}
	data := buildWAL(t, want[0])
	stop := len(data)

	frame := make([]byte, 8)
	binary.LittleEndian.PutUint32(frame[4:], wal.MaxPayload+1)
	data = append(data, frame...)

	var got [][]byte
	f := &fakeFile{data: data}
	offset, err := wal.Replay(f, int64(len(data))+wal.MaxPayload+100, func(payload []byte) error {
		got = append(got, slices.Clone(payload))

		return nil
	})
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if !equalPayloads(got, want) {
		t.Errorf("got %d payloads, want %d", len(got), len(want))
	}
	if offset != int64(stop) {
		t.Errorf("got offset %d, want %d", offset, stop)
	}
	if f.reads != 4 {
		t.Errorf("got %d reads, want 4: the oversized length must be rejected before its payload is read", f.reads)
	}
}

func TestReplay_PropagatesApplyError(t *testing.T) {
	t.Parallel()

	data := buildWAL(t, []byte("alpha"), []byte("bravo"), []byte("charlie"))
	secondRecord := len(buildWAL(t, []byte("alpha")))

	var applied int
	f := &fakeFile{data: data}
	offset, err := wal.Replay(f, int64(len(data)), func([]byte) error {
		applied++
		if applied == 2 {
			return errBoom
		}

		return nil
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("got error %v, want %v", err, errBoom)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("offset %d", secondRecord)) {
		t.Errorf("got error %q, want it to name offset %d", err, secondRecord)
	}
	if applied != 2 {
		t.Errorf("got %d applied records, want 2", applied)
	}
	if offset != int64(secondRecord) {
		t.Errorf("got offset %d, want %d", offset, secondRecord)
	}
}

func TestReplay_WrapsReadErrors(t *testing.T) {
	t.Parallel()

	data := buildWAL(t, []byte("alpha"), []byte("bravo"))
	firstRecord := 12

	tests := []struct {
		name          string
		giveReadsOK   int
		wantOffset    int64
		wantErrSubstr string
	}{
		{
			name:          "header read fails",
			giveReadsOK:   0,
			wantOffset:    0,
			wantErrSubstr: "read header",
		},
		{
			name:          "record header read fails",
			giveReadsOK:   1,
			wantOffset:    int64(firstRecord),
			wantErrSubstr: "read record header",
		},
		{
			name:          "record payload read fails",
			giveReadsOK:   2,
			wantOffset:    int64(firstRecord),
			wantErrSubstr: "read record payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeFile{data: data, readErr: errBoom, readErrAfter: tt.giveReadsOK}
			offset, err := wal.Replay(f, int64(len(data)), func([]byte) error { return nil })
			if !errors.Is(err, errBoom) {
				t.Fatalf("got error %v, want %v", err, errBoom)
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("got error %q, want it to contain %q", err, tt.wantErrSubstr)
			}
			if offset != tt.wantOffset {
				t.Errorf("got offset %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}
