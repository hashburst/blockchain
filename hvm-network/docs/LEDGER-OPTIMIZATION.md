# HVM Network: bounded ledger and measured recovery

Status: development candidate. These primitives do not yet replace the resident
`Blockchain.Blocks` slice, the existing checkpoint writer, or peer bootstrap.
No mainnet activation, legacy balance migration, or live node restart is included.

## Storage identity and protocol compatibility

Base commit: 93f97439206aeb37f516c8a3c8909b762b15c118.
The running HVM ledger is `blockchain.dat` with `blockchain.idx`. References to
`masterData.hbx` also exist in legacy documentation/PHP, including encrypted JSON.
An existing legacy file MUST NOT be reinterpreted as HBX2. A future converter must
write a separate directory, verify every decoded block/hash and state commitment,
fsync it, then switch the selected storage generation explicitly. Retain originals.

`blockchain/ledger_codec.go` uses the actual Block, Transaction, TransactionV2,
APoWProof, PrevoteCertificate and QuorumCertificate types. It preserves all current
fields, including EVM raw transactions, BFT votes and signatures, economic values,
metadata and proof parameters. Float64 legacy amounts are stored as IEEE bits;
this codec does not change their economic semantics. Timestamps retain their
instant and normalize to UTC. No consensus hashing or signing bytes are changed.
HBX2/HBB2 are versioned storage formats only. JSON remains appropriate for human
configuration and RPC; removing JSON there is not this optimization.

## Package layout and ownership

- `ledger/segment_unix.go`: read-only mmap on Linux/macOS using syscall.Mmap.
  The standard syscall package is frozen; a later switch to x/sys/unix should keep
  identical lifetime contracts. The file must be a sealed immutable inode.
- `ledger/segment_other.go`: bounded ReadAt fallback, including Windows. No claim
  of zero-copy or platform-tested directory fsync on that fallback.
- `ledger/indexed.go`: indexed lazy reads by ordinal, maximum eight active reads,
  byte/entry-limited cache, short-lived scratch pooling. A production segment
  catalog must bound open mappings as well as cache memory.
- `ledger/cache.go`: mutex-protected LRU. Hits change recency and therefore use
  Lock, while Stats uses RLock. Encoded values are immutable internal copies.
- `ledger/frame.go`: bounded binary frame, CRC32C corruption detection. CRC is not
  authenticity; validate consensus hash and signatures separately.
- `ledger/checkpoint*.go`: one background writer, one pending job, local HMAC,
  gzip BestSpeed, fsync temporary file, atomic rename, fsync directory.
- `blockchain/ledger_codec.go`: reflection-free manual storage encoding/decoding.
  The pool reuses Block shells, clears references before return, and does not
  recycle a pointer while a reader owns it. Nested decoding still allocates.
- `internal/diagnostics`: bounded native Go profiling with no network listener.
- `deploy/diagnostics/sample-node.py`: read-only /proc and cgroup sampler.

Example integration into an archive consumer (not a live store migration):

```go
reader, err := ledger.OpenIndexed(dataPath, indexPath, 256<<20, 8<<20, 256)
if err != nil { return err }
defer reader.Close()
return reader.WithPayload(ordinal, func(encoded []byte) error {
    return blockchain.WithLedgerBlock(encoded, func(b *blockchain.Block) error {
        // Consume within this callback. Never retain pooled/mapped references.
        return verifyBlock(b)
    })
})
```

The mmap callback lends encoded bytes without a payload copy on a cache miss;
cache insertion and decoding do allocate. Cache hits copy into reusable owned
scratch so evictions cannot invalidate a caller's memory. For sequential scans,
`ReadFrame(bufio.NewReaderSize(file, 256<<10), scratch)` avoids repeated tiny reads.
`io.NewSectionReader` supports indexed ReadAt without a shared file-position lock.

Mmap reduces Go heap allocation, not necessarily RSS, page faults or total RAM.
OS page cache and mapped resident pages must be measured. Never truncate a mapped
inode: it can cause SIGBUS rather than a recoverable Go error. Replace sealed
segments by inode publication and retire old mappings after their reader leases.

## Async checkpoints and durability

A checkpoint is a derived optimization. A signed vote/lock journal and canonical
block/WAL must be durable BEFORE the node acknowledges the relevant durable state.
Calling Sync only after an occasional checkpoint is not sufficient for consensus.

The provided writer accepts a frozen encoded payload, copies it for ownership,
and performs compression and filesystem publication in the background. The caller
must first capture a coherent version of state. A shallow copy of mutable maps,
EVM StateDB, receipt logs or byte slices is not an isolated snapshot.

For runtime integration: pin an immutable state root under the state lock; retain
its persistent COW trie/database generation; release the lock; serialize and
compress that pinned generation; publish the authenticated checkpoint and release
the pin. Native balances, HVM storage, validator registry, receipts and EVM state
must all identify the same finalized height. Garbage collection of trie nodes must
respect outstanding snapshot pins. This state-root pinning is not implemented by
the standalone byte writer and remains a required integration gate.

Use one writer per directory. Parallel compression/writers can reorder generation
publication and increase memory/CPU pressure. TrySubmit rejects an over-budget or
full queue; skipping an optional checkpoint is allowed, dropping canonical blocks
or transactions is not. Close drains accepted jobs; callers must collect completion
errors. Copying the payload takes O(snapshot size), so neither zero contention nor
zero finalization overhead is promised. Crash tests on real filesystems must cover
power loss before/after file Sync, rename and directory Sync, not just process exit.

## Removing resident history: required runtime integration

Replace direct `Blocks[index]`, append, length and range access with a BlockStore
interface (Height, Read, AppendFinalized, Scan). Keep only a bounded head/round
working set in memory. Stream replay in ascending order, execute state transitions
sequentially, and keep the historical RPC window explicitly bounded. RPC and P2P
must not reconstruct an unbounded []Block response. Bounds must include decoded
objects, in-flight RPCs, open mappings, snapshot generations and the EVM trie cache.

The initial package provides the reader boundary, not this complete refactor.
The existing runtime has many direct Blocks accesses. Replacing only LoadAll would
break those invariants. Switch behind an explicit storage version after end-to-end
replay, RPC, finality and recovery equivalence tests against existing storage.

## Authenticated bootstrap for NEW nodes

A local checkpoint HMAC is meaningful only to its originating node. Do not send
that key to peers and do not accept a snapshot signed by an arbitrary peer.

A bootstrap protocol must:

1. Start from an independently trusted network/genesis/configuration anchor and
   validator-set root. Never trust the snapshot's own proposed validator set.
2. Verify validator-set transitions and the finalized block's weighted quorum
   certificate using the existing consensus signature/domain checks. Include the
   chain ID and protocol activation configuration. Account for validator rotation
   and the assumed freshness/trust model; a self-contained signed file is not enough.
3. Obtain a manifest whose canonical digest commits height, block hash, all state
   roots, chunk lengths/hashes, codec version, total expanded bytes and tail range.
4. Bound compressed and expanded bytes; verify every chunk before importing it
   into a separate temporary store. Recompute native/HVM/EVM/validator state roots
   and compare them with the authenticated finalized block, not just the manifest.
5. Replay and validate the recent suffix; preserve the EVM historical API window.
6. Publish the new store atomically after all checks. New validators get their own
   identities and local signing journals; never clone another validator's journal.

The current block certificates do not themselves authenticate an arbitrary archive
manifest. Their block/state roots can anchor a manifest only when imported state
is independently recomputed to those roots. TEP is transport, not trust. This
network bootstrap is specified here, not implemented or certified by this PR.

## PoH, APoW and CPU allocation

The existing PoH recurrence uses SHA-512 over an eight-byte state, copies the first
eight digest bytes, and repeats. Keep that recurrence and tick count unchanged.
Fixed stack arrays already avoid allocating every tick. Independent proofs may be
checked by a small bounded worker pool; the dependent steps inside one PoH chain
cannot be parallelized without changing the protocol. Verify whether CPU dispatch
and compiler assembly help this exact 8-byte workload using a benchmark on each
CPU; do not assume a particular instruction extension or a speedup.

A small exact-input PoH cache exists already. Profile it before increasing it or
adding another cache. In-flight duplicate calculations may later be coalesced if
profiles show material duplication; cache misses must still compute the proof.

APoW mining stays in a separate process/service. runtime.Gosched merely yields a
Go goroutine; context cancellation is cooperative; neither assigns OS priorities.
Use cgroup CPU quotas/weights and optional disjoint CPU affinity for miner and
validator, with bounded miner workers and cancellation when the work head changes.
Avoid CPU-burning polling; use notifications and backoff. An RL controller may
select local resource budgets within policy, but must not change consensus target,
reward or validity rules independently. Its integration was not established by
this storage review. No universal CPU quota is justified before measuring the VPS.

## Measurement plan and acceptance

Observed v2: mode=full, checkpoint_height=-1, replay_blocks=141130,
elapsed_ms=7019892. This is wall time, not measured CPU time or energy. v3's shorter
full recovery is not an apples-to-apples performance benchmark across hardware.

Collect CPU, heap/allocations, block, mutex and execution trace in separate runs or
windows. Profilers perturb execution; do not enable all of them at maximum sampling.
Measure RSS and mapped pages independently of Go heap; include cgroup throttling,
CPU steal, I/O counters/pressure, restart count and the same finalized dataset.

Use pprof alloc_space vs inuse_space to distinguish allocation churn from retention.
Block/mutex profiles measure synchronization, not all disk/network I/O; pair them
with runtime trace and OS counters. A process sleeping in futex is not proof of a
consensus deadlock. A Python worker waiting for a child does not profile the child.

Record cold replay and warm checkpoint recovery separately, with binary commit,
Go version, CPU/quota, dataset digest/height, checkpoint height, elapsed time,
CPU-seconds, peak RSS, allocated bytes, GC CPU, read/write bytes and p50/p95/p99
finalization latency. Repeat at least three times under comparable load. Check
all finality roots/receipts/journal prefixes and enforce a bounded queue under load.

Energy requires power measurements (host RAPL where available, BMC or external
meter). VPS CPU time is only a proxy: it cannot establish Joules or carbon savings.
Report Joules per verified block and per recovery with idle baseline and machine
boundaries. Less GC allocation may reduce CPU work, but mmap page faults,
compression and additional copies may offset the benefit. Measure rather than
claiming zero GC or guaranteed energy savings.

## Official references

- https://go.dev/doc/diagnostics
- https://go.dev/wiki/Performance
- https://pkg.go.dev/runtime/pprof
- https://pkg.go.dev/runtime/trace
- https://pkg.go.dev/sync#Pool
- https://pkg.go.dev/golang.org/x/exp/mmap (ReadAt API copies to caller buffer)

## Release gates

This candidate has no automatic installer. First profile one node during a planned
restart using the existing journal-preserving updater after validating its exact
binary/override. Never start a second process on the live data directory. Preserve
all consensus journal, recovery lock and pin checks. Then integrate/test bounded
BlockStore and COW state capture, run cold/warm/crash/fault tests, and roll out one
node at a time. Five-node common-height agreement, APoW rewards, finality progress,
restart recovery, and MetaMask/API checks precede mainnet. Mainnet additionally
requires separate state/identity/economics and verified legacy spending closure.
