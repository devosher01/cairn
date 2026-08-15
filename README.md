# cairn

![ci](https://github.com/devosher01/cairn/actions/workflows/ci.yml/badge.svg)
![nightly](https://github.com/devosher01/cairn/actions/workflows/nightly.yml/badge.svg)

An embedded, persistent, ordered key-value storage engine for Go, built on a
log-structured merge-tree, with zero third-party dependencies.

cairn runs inside a single process. Writes are serialized internally by one
writer, so ordering is a property of the engine rather than of the caller;
readers scale across goroutines and observe immutable MVCC snapshots that are
unaffected by concurrent writes. A batch commits atomically: it is visible in
full or not at all, both to concurrent readers and to recovery after a crash.
Iteration is in ascending bytewise key order.

Crash safety is the design center rather than a feature. After any crash,
recovery yields a state equal to a prefix of the acknowledged operation
history — no holes, no reordering, no partially applied batch, no corruption —
and under the default sync mode that prefix includes every acknowledged write.
That claim is not an aspiration: the crash space is enumerated mechanically on
every run of the test suite, and [How it is tested](#how-it-is-tested)
describes the machinery that does it.

cairn requires Go 1.26.5 or newer, the version pinned in `go.mod`.

## Quick start

```sh
go get github.com/devosher01/cairn
```

Open a database on a directory, write and read single keys, and close it. A
`nil` `*Options` selects the defaults, which include fsync on every commit.

```go
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/devosher01/cairn"
)

func main() {
	db, err := cairn.Open("/var/lib/example", nil)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Put([]byte("summit"), []byte("2743")); err != nil {
		log.Fatal(err)
	}

	value, err := db.Get([]byte("summit"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("summit = %s\n", value)

	if err := db.Delete([]byte("summit")); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Get([]byte("summit")); !errors.Is(err, cairn.ErrNotFound) {
		log.Fatalf("expected the key to be gone, got %v", err)
	}

	if err := db.Close(); err != nil {
		log.Fatal(err)
	}
}
```

`Get` returns a copy that the caller owns, and reports `ErrNotFound` for both
an absent key and a deleted one.

A `Batch` collects puts and deletes; `Write` commits the whole batch under one
sequence of numbers and one WAL record.

```go
func writeAtomically(db *cairn.DB) error {
	b := cairn.NewBatch()
	b.Put([]byte("bravo"), []byte("2"))
	b.Put([]byte("charlie"), []byte("3"))
	b.Delete([]byte("alpha"))

	return db.Write(b)
}
```

An iterator walks a half-open key range, `LowerBound` inclusive and
`UpperBound` exclusive; a `nil` bound is unbounded on that side. `First`,
`SeekGE`, and `Next` report whether the iterator landed on an entry, and
`Error` reports any I/O or corruption error that ended the walk early.

```go
func scanRange(db *cairn.DB) error {
	it, err := db.NewIterator(cairn.IterOptions{
		LowerBound: []byte("b"),
		UpperBound: []byte("d"),
	})
	if err != nil {
		return err
	}
	defer it.Close()

	for ok := it.First(); ok; ok = it.Next() {
		fmt.Printf("%s = %s\n", it.Key(), it.Value())
	}

	return it.Error()
}
```

`Key` and `Value` return memory owned by the iterator and valid only until the
next positioning call; copy anything that has to outlive it.

`Flush` forces the memtable to disk and returns once it is durable; `Compact`
additionally drains all pending compaction work. Neither is needed in normal
operation — the background worker schedules both — but they are useful before a
backup or after a bulk load.

A snapshot pins a point-in-time view. Reads through it ignore every write
committed after it was taken, and it holds the underlying files alive until it
is closed.

```go
func readPinned(db *cairn.DB, key []byte) ([]byte, error) {
	snap, err := db.NewSnapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Close()

	return snap.Get(key)
}
```

Snapshots and iterators are handles on engine state, so `Close` on the database
reports `ErrOpenHandles` while any of them are still open. Runnable versions of
all of the above live in [`example_test.go`](example_test.go) and render as
examples on pkg.go.dev.

## Durability

The sync mode is the only durability knob, and it is chosen once at `Open`.

| Mode | Contract on `Write` return |
|---|---|
| `SyncAlways` (default) | Entry is durable. A crash loses nothing acknowledged. |
| `SyncInterval` (window `Options.Interval`, 100 ms by default) | Entry is durable within at most the interval, fsynced by a background timer. A crash loses at most the last window. |
| `SyncOff` | Durable only at WAL rotation and `Close`. A crash may lose the entire unsynced suffix. |

All three modes preserve the prefix property. What varies between them is how
much of the acknowledged tail a crash may take, never whether the survivors
form a prefix: WAL rotation and `Close` fsync unconditionally in every mode, so
the durable frontier never interleaves across files, and no crash can leave a
hole in the middle of the recovered history or a half-applied batch at its end.

If a WAL append, a WAL fsync, or a manifest install fails, the database enters a
sticky failed state: every later write returns that error wrapped in
`ErrDBFailed`, while reads may continue. After a failed fsync the durable
frontier is unknowable, so cairn stops claiming otherwise rather than writing
on top of an unknown state.

The remaining options tune sizes and thresholds, and every one of them has a
default:

| Option | Default | Effect |
|---|---|---|
| `MemtableSize` | 4 MiB | Memtable size that triggers rotation and a flush. |
| `BlockSize` | 4 KiB | Data block cut target inside an SSTable. |
| `BloomBitsPerKey` | 10 | Bloom filter budget, roughly a 1% false-positive rate. |
| `L0CompactTrigger` | 4 | L0 table count that scores 1.0 and schedules a compaction. |
| `L0StallTrigger` | 12 | L0 table count at which writers block until compaction catches up. |
| `BaseLevelSize` | 10 MiB | L1 byte target; each deeper level targets ten times the one above. |
| `TargetFileSize` | 4 MiB | Size at which compaction cuts an output table. |
| `DisableAutoCompaction` | false | Stops the background worker so a test can drive flush and compaction itself. |
| `Env` | real | Injection point for the deterministic simulation described below; leave it zero to use the real filesystem, clock, and randomness. |

`db.Metrics()` returns a snapshot of counters: puts, deletes, gets, WAL bytes
written, flushes and flushed bytes, compactions with bytes read and written,
write stalls, and per-level table counts and sizes. Write amplification is
derived from them rather than stored, as
`(WALBytesWritten + FlushBytes + CompactionBytesWritten)` over user bytes
written.

## Architecture

A write is appended to the write-ahead log, fsynced according to the sync mode,
and applied to a skiplist memtable. When the memtable fills, it becomes
immutable, a fresh WAL and memtable take over, and a single background worker
writes the immutable memtable out as one L0 SSTable. Leveled compaction then
merges L0 into L1 and each `Ln` into `Ln+1`, where L1 through L6 hold sorted,
pairwise-disjoint tables. Every SSTable carries a bloom filter over its user
keys and a block index, and every block, footer, WAL record, and manifest
carries a CRC32-C.

A read consults the memtable, then the immutable memtable, then the L0 tables
newest first, then at most one table per level below, taking the first entry
whose sequence number is visible to the reader. Iterators are a k-way merge over
those same sources. Readers hold a lock only long enough to take references;
all I/O then runs against immutable structures, so reads never block writes.

The persistent file set lives in a single `MANIFEST`, rewritten in full and
installed by fsync, rename, and directory fsync. A crash therefore lands on the
old manifest or the new one, never on a mixture, and files a manifest no longer
references are deleted only after that manifest is durable. A database directory
holds `LOCK`, `MANIFEST`, and numbered `NNNNNN.wal` and `NNNNNN.sst` files, and
nothing else.

[DESIGN.md](DESIGN.md) is the reference for the byte-level formats, the recovery
state machine, the crash analysis by boundary, the concurrency model, and the
rationale behind each of these choices.

## How it is tested

Everything nondeterministic — the filesystem, the clock, the random source — is
injected through one `Env` interface. The engine never touches `os`, `time`, or
a global random source directly, which a lint check in CI enforces on every
file. Swapping in a simulated environment therefore makes an entire database,
including its background worker and its flush and compaction scheduling, a pure
function of a seed.

**Deterministic simulation.** The simulated filesystem keeps a durable image and
a volatile write journal per file, plus a volatile journal of directory entries,
since a create or a rename is not durable until the directory is fsynced. Every
mutating call is appended to an operation log. Crashing the simulation at any
index in that log materializes the disk a real crash could have left, under one
of three crash modes: `none`, where no unsynced write survives; `prefix`, where
unsynced writes survive up to a cut point with the cut write torn at an
enumerated byte boundary; and `scatter`, where a seeded arbitrary subset of
unsynced 512-byte sectors survives, modeling out-of-order writeback. Directory
operations survive or vanish independently of file writes. On top of crashes, the
simulation injects `EIO` on write and fsync, `ENOSPC` at a chosen byte budget,
and rename failures.

**Complete crash-space enumeration.** A campaign workload runs once against the
simulation to record its operation log, and then every index in that log is
crashed, in every mode, with every torn cut. Each of those disks is recovered and
checked three ways: recovery has to succeed without corruption or panic, the
recovered state has to equal a prefix of the oracle's acknowledged history — one
that includes every acknowledged write under `SyncAlways` — and all structural
invariants have to hold. Each CI run enumerates about 27,000 crash states across
two workloads, one per sync mode. The nightly job scales the seed count
eightfold, to roughly 216,000 crash states per operating system, on Linux and
macOS.

**Model-based testing.** A seeded generator produces sequences over a small key
domain drawn from puts, deletes, batches, gets, range scans, snapshot creation,
snapshot reads and scans, flushes, compactions, reopens, and crash-and-recover.
Each sequence runs against cairn on the simulation and against a trivial oracle
built from an ordered map, with every read compared as it happens and a full
iterator walk compared against the oracle dump at the end. Disabling automatic
compaction and generating explicit flush and compaction operations makes even
the background scheduling part of the deterministic sequence. A CI run executes
51,000 operations across 340 sequences in the three sync modes; the nightly job
scales that fortyfold, to roughly two million operations per operating system,
and adds a seed derived from the run so that each night explores sequences no
earlier run has seen. Failures print the seed that reproduces them.

**Structural invariants.** Eight invariants are checked after every flush,
compaction, and recovery in all of the harnesses: every manifest-referenced file
exists with the recorded size (I1); L1 through L6 are sorted and pairwise
disjoint (I2); internal keys ascend strictly within a table and match the
manifest's recorded bounds and entry count (I3); every block and footer CRC
verifies (I4); bloom filters produce no false negative for their own keys (I5);
no entry's sequence number exceeds the manifest's last sequence (I6);
`next_file_num` exceeds every file number on disk (I7); and no file is deleted
while a live version references it, nor does any orphan survive recovery or a
clean close (I8).

**Fuzzing and the race detector.** Five fuzz targets cover the decode paths that
face untrusted bytes — batch reader, SSTable open, manifest decode, WAL replay —
plus the operation-sequence harness itself; each must return a corruption error
or valid data, and never panic. CI runs a short budget per target and the nightly
job runs ten minutes each. Every test run in CI, on Linux and macOS, uses
`-race`.

**Real kill -9.** The simulation proves the engine's protocol; it cannot prove
that `osenv` and the operating system behave as modeled. A harness binary writes
with `SyncAlways` to a real filesystem and streams each acknowledged key to its
parent over a pipe. The parent kills it with SIGKILL at a random moment, reopens
the database, and verifies that every acknowledged key is present with the right
value and that the invariants hold. It is non-deterministic by design and
complements the simulation rather than replacing it: eight iterations per CI run
on both operating systems, 150 nightly.

Two findings during development are the reason to believe the machinery works
rather than merely runs. The first was a modeling gap: crash materialization
skipped failed operations, so an `ENOSPC` write that had already put some bytes
on disk showed up on the live filesystem but never in any post-crash image,
which would have hidden a class of partial writes the engine has to survive.
Failed writes with partial data now participate as volatile writes. The second
was a real engine bug, and only `scatter` mode could find it: a new SSTable's
directory entry and the manifest rename were both waiting on the same directory
fsync, so out-of-order metadata writeback could persist a manifest referencing a
table whose directory entry had not landed — a database that recovers into a
missing file. The campaign caught it at seed 7100, operation 207. The fix fsyncs
the directory after the new tables and before installing the manifest, in both
flush and compaction.

## Benchmarks

Measured on an Apple M3 Pro, `darwin/arm64`, Go 1.26.5, on 2026-08-15. Each
figure is the median of five samples; the raw output is committed at
[`bench/results/apple-m3-pro-2026-08-15.txt`](bench/results/apple-m3-pro-2026-08-15.txt).
To reproduce:

```sh
cd bench
go test -bench . -benchmem -count 5 ./...
```

Lower is better throughout. Keys are 16 bytes, values 100 bytes, and every
engine runs with its own default configuration against the identical workload.

| Benchmark | cairn | bbolt | Pebble |
|---|---|---|---|
| `FillSeq`, sequential put, no fsync (µs/op) | 3.66 | 25.8 | 0.73 |
| `FillSync`, sequential put, fsync per write (ms/op) | 2.66 | 5.95 | 2.68 |
| `FillRandom`, random put, no fsync (µs/op) | 4.12 | 23.5 | 1.40 |
| `ReadRandom`, random point read (µs/op) | 2.51 | 0.89 | 2.91 |
| `Scan`, one full pass over 200,000 entries (ms/op) | 13.9 | 1.58 | 16.8 |

A separate amplification workload loads 300,000 keys with 512-byte values, about
151 MiB of user data, in random key order with `SyncOff`, and reads back
`Metrics()`:

```sh
cd bench
go test -run TestAmplification -v ./...
```

It reports a write amplification of 5.4 and a space amplification of 1.01, both
measured once the background worker has drained. Sampled immediately after the
load returns, while compaction is still running, the same workload reads 5.1 and
0.99; neither sample is a steady state, and the honest reading is the pair.

Reading the table: on synced writes cairn and Pebble are within one percent of
each other, because both pay one `F_FULLFSYNC` per write and the device sets the
price, while bbolt's fully durable single-key transaction costs a page rewrite
plus a meta page write and lands at roughly twice that. With fsync off, cairn's
buffered writes sit between the two, several times faster than a bbolt
transaction per put and several times slower than Pebble. On point reads and
scans the B-tree wins outright: bbolt hands back a pointer into a memory-mapped
page, where cairn decodes and checksums a 4 KiB block per read — about 5 KB of
allocation per read in this benchmark — and merges across levels for a scan.
Version 1 ships without group commit, without a block cache, and without an
arena allocator for the memtable, which is the short explanation for the write
gap, the read gap, and cairn's allocation counts respectively; all three are
[future work](DESIGN.md#19-future-work-explicitly-out-of-v1), and each waits for
a measured justification.

These numbers belong to the machine that produced them. A storage engine
benchmark measures a device, a filesystem, and a CPU at least as much as it
measures code, and the fsync-bound rows will disagree by an order of magnitude
on a cloud VM with a network disk. [`bench/README.md`](bench/README.md) documents
each workload and the fairness rules behind the comparison.

## Guarantees and limits

Keys are 1 to 65536 bytes and are ordered by `bytes.Compare`. Values are 0 to
4 MiB, where a nil value and an empty one are equivalent. A batch holds at most
1,000,000 entries and encodes to at most 64 MiB. Every limit is validated at the
public API, which reports `ErrInvalidKey`, `ErrInvalidValue`, or
`ErrBatchTooLarge`; all errors are wrapped with context and match under
`errors.Is`.

A database directory belongs to one process. `Open` takes an exclusive advisory
lock and a second `Open` on the same directory reports `ErrLocked`, which makes
cairn unsuitable for sharing a directory between processes. `DB` itself is safe
for concurrent use by any number of goroutines; an `Iterator` and a `Batch` are
single-goroutine values. Calls after `Close` report `ErrClosed`.

Version 1 deliberately does not attempt networking, replication, SQL,
multi-key transactions beyond the atomic batch, block compression, a block
cache, custom comparators, TTLs, or column families. Nothing in the engine
speculates about them. The items cairn expects to gain, once a measurement
justifies each of them, are listed as
[future work](DESIGN.md#19-future-work-explicitly-out-of-v1).

## License

MIT.
