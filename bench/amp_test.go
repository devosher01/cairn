package bench

import (
	"testing"

	"github.com/devosher01/cairn"
)

const (
	_ampKeys     = 300_000
	_ampValueLen = 512
	_ampGets     = 50_000
)

func TestAmplification(t *testing.T) {
	if testing.Short() {
		t.Skip("amplification loads ~150MB of user data")
	}

	dir := t.TempDir()
	db, err := cairn.Open(dir, &cairn.Options{Sync: cairn.SyncOff})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	pool := newValuePool(_seedAmpValues, _ampValueLen)
	key := make([]byte, _keyLen)
	for _, n := range shuffledKeys(_ampKeys, _seedAmpOrder) {
		fillKey(key, uint64(n))
		if err := db.Put(key, pool.at(uint64(n))); err != nil {
			t.Fatalf("put: %v", err)
		}
	}

	userBytes := uint64(_ampKeys) * uint64(_keyLen+_ampValueLen)
	reportAmplification(t, "after load", db.Metrics(), userBytes)

	for _, n := range randomKeys(_ampGets, _ampKeys, _seedAmpReads) {
		fillKey(key, uint64(n))
		if _, err := db.Get(key); err != nil {
			t.Fatalf("get: %v", err)
		}
	}

	reportAmplification(t, "after gets", db.Metrics(), userBytes)
}

func reportAmplification(t *testing.T, label string, m cairn.Metrics, userBytes uint64) {
	t.Helper()

	var levelBytes int64
	for level, lm := range m.Levels {
		if lm.Tables == 0 {
			continue
		}
		levelBytes += lm.Bytes
		t.Logf("%s: L%d tables=%d bytes=%d", label, level, lm.Tables, lm.Bytes)
	}
	t.Logf("%s: puts=%d walBytes=%d flushes=%d flushBytes=%d", label,
		m.Puts, m.WALBytesWritten, m.Flushes, m.FlushBytes)
	t.Logf("%s: compactions=%d compactionRead=%d compactionWritten=%d stalls=%d", label,
		m.Compactions, m.CompactionBytesRead, m.CompactionBytesWritten, m.WriteStalls)

	written := m.WALBytesWritten + m.FlushBytes + m.CompactionBytesWritten
	t.Logf("%s: userBytes=%d writtenBytes=%d liveBytes=%d writeAmp=%.2f spaceAmp=%.2f", label,
		userBytes, written, levelBytes,
		float64(written)/float64(userBytes), float64(levelBytes)/float64(userBytes))
}
