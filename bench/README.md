# cairn benchmarks

A separate Go module so that the engine itself stays dependency-free. Nothing
here is imported by `github.com/devosher01/cairn`; the third-party comparison
engines — [bbolt](https://go.etcd.io/bbolt) and
[Pebble](https://github.com/cockroachdb/pebble) — exist only in this module's
`go.mod`. The module uses a `replace` directive, so it always measures the
working tree next to it, never a published version.

## Running

```sh
cd bench
go test -bench . -benchmem -count 5 ./...
```

`-count 5` gives benchstat enough samples to compute a confidence interval.
To compare two revisions, save both runs and diff them:

```sh
go test -bench . -benchmem -count 5 ./... > old.txt
# ...change the engine...
go test -bench . -benchmem -count 5 ./... > new.txt
go run golang.org/x/perf/cmd/benchstat@latest old.txt new.txt
```

A single benchmark with a fixed iteration count (useful when a full run is too
slow, e.g. the fsync-bound one):

```sh
go test -run '^$' -bench BenchmarkFillSync -benchtime=300x -benchmem ./...
```

Use `-benchtime=Nx` for smoke tests only, never for numbers worth quoting. A few
hundred iterations sit entirely inside every engine's write buffers, which
flatters whichever engine buffers most and never pays for a flush or a
compaction; the default time-based `-benchtime` runs long enough for that cost
to land inside the measurement.

The amplification test is a plain test, not a benchmark, and is skipped under
`-short`:

```sh
go test -run TestAmplification -v ./...
```

## What each benchmark measures

Every benchmark runs as three sub-benchmarks — `cairn`, `bolt`, `pebble` —
against the same workload, with a fresh database in a fresh `b.TempDir()` per
sub-benchmark. Keys are 16 bytes, `%016d`; values are 100 bytes; `b.SetBytes`
on the fill benchmarks is therefore 116, so `MB/s` counts user bytes, not
bytes on disk.

| Benchmark | Workload |
|---|---|
| `BenchmarkFillSeq` | Sequential keys, one put per operation, no fsync. The best case for every engine: keys arrive in order. |
| `BenchmarkFillSync` | Same, with fsync-per-write durability. Measures the storage device more than the engine; expect milliseconds per op. |
| `BenchmarkFillRandom` | Keys drawn uniformly at random from a 1,048,576-key space (seeded, so every engine sees the same sequence). Exercises the write path with no locality. |
| `BenchmarkReadRandom` | 200,000 sequential keys preloaded, then the store is closed and reopened, then 1,000 keys are read to warm caches; the timed loop reads a seeded random key each iteration. Every read must hit — a miss fails the benchmark. |
| `BenchmarkScan` | 200,000 keys preloaded, closed and reopened; each iteration is one full forward scan that must see exactly 200,000 entries. `b.SetBytes` is the whole dataset, so `MB/s` is scan throughput. |

`TestAmplification` (`amp_test.go`) loads 300,000 keys × 512-byte values
(~151 MiB of user data) into cairn with default options and no fsync, in a
seeded random key order with each key written exactly once, then logs
`Metrics()` twice: once right after the load, and once after 50,000 random
reads. It reports

- write amplification = `(WALBytesWritten + FlushBytes + CompactionBytesWritten) / userBytes`
- space amplification = `(sum of per-level table bytes) / userBytes`

with `userBytes` the sum of key and value lengths written. Two samples are
logged because background compaction is still draining when the load returns:
the first sample understates the eventual compaction cost, the second is taken
after the reads have given the background worker time to catch up. Neither is a
steady state — the honest reading is the pair.

## Fairness notes

- **Default configuration everywhere.** No engine gets a tuned cache, block
  size, memtable size, or compaction knob. cairn runs with `cairn.Options{Sync:
  …}` and nothing else; Pebble runs with `&pebble.Options{}`; bbolt runs with
  `&bbolt.Options{NoSync: …}`. Whatever each project ships as its default is
  what is measured.
- **Sync semantics differ by engine, by design.** With sync on, cairn uses
  `Sync: SyncAlways` (one WAL fsync per write), Pebble uses
  `&pebble.WriteOptions{Sync: true}` (one WAL fsync per write), and bbolt runs
  in its natural fully durable mode. With sync off, cairn uses `SyncOff`,
  Pebble uses `WriteOptions{Sync: false}`, and bbolt sets `NoSync: true`.
  On macOS all three end up in `os.File.Sync`, which is `fcntl(F_FULLFSYNC)`,
  so the fsync comparison is apples-to-apples at the syscall level.
- **bbolt commits a transaction per put.** The `store.put` contract is one
  durable single-key write, and in bbolt that is a `db.Update` transaction:
  a B+tree page rewrite plus a meta page write, i.e. two fsyncs in the sync
  case. That is bbolt's nature, not a handicap imposed here — batching many
  puts into one transaction would measure a different operation than the one
  cairn and Pebble are performing. Read it as "single-key write cost", and read
  bbolt's scan number (where it is by far the fastest) as the other side of the
  same design.
- **Reads return an owned copy in all three adapters.** cairn's `Get` clones by
  contract; the bbolt and Pebble adapters clone too, instead of handing back a
  slice into an mmap or a pinned block. Without this, cairn would be charged for
  an allocation the others avoid only by returning borrowed memory.
- **Values are incompressible.** Every value is a distinct window into a 1 MiB
  buffer of seeded random bytes. Pebble compresses SSTables by default and
  cairn does not; zero-filled or repeated values would have handed Pebble a
  large, meaningless I/O advantage.
- **Both stores are reopened before read benchmarks.** After the preload, every
  engine is closed and reopened, so no engine is measured with a hot memtable
  full of data that another engine has already flushed.
- **Pebble's info logging is silenced.** `Options.Logger` is a small logger that
  drops `Infof` and forwards `Errorf`/`Fatalf` to `pebble.DefaultLogger`. This
  is not tuning — it keeps a stray "Found 0 WALs" line out of the middle of the
  benchmark output, where it would confuse benchstat. Errors still surface.
- **Deterministic workloads.** All key orders come from `math/rand/v2` with
  fixed seeds, so every engine sees the identical sequence and reruns are
  comparable.

## These numbers belong to the machine that produced them

Storage engine benchmarks measure a device, a filesystem, and a CPU at least as
much as they measure code. A run on a laptop with APFS on NVMe, a run on a cloud
VM with a network disk, and a run in a container with a throttled I/O budget
will disagree by an order of magnitude, especially on the fsync-bound
benchmarks. Never quote a number from this directory without stating the machine
and the OS it came from, and never compare a number from one machine against a
number from another.
