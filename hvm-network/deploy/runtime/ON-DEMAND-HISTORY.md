# Persistent HVM history on demand

The ordinary testnet/mainnet state opener and the offline verifier now read the
existing `blockchain.dat` / `blockchain.idx` pair through indexed history. They
leave `Blockchain.Blocks` empty. No filename, consensus rule, identity, balance,
block hash or signing journal changes. Both existing Gob frames and converted
HBX2/HBB2 binary frames are supported. The separate legacy constructor retains
its compatibility slice and legacy fork-choice behavior.

## Ownership and bounds

The index remains 20-byte big-endian `[uint64 ordinal][uint64 offset][uint32 frame
size]`. Open validates contiguous index/frame layout and rejects truncation,
mixed formats and unindexed tails. Full replay streams blocks; it does not
materialize the complete decoded history. Checkpoint recovery uses the same
reader. `BlockAt` returns caller-owned values; a failed disk/decode lookup is not
a missing block or a zero EVM hash. Network range requests use the checked API.

Persistent history retains one head, 257 immutable height/hash entries for EVM
ancestor lookups, and at most 8 MiB / 512 entries of encoded cache. Decoder work
has four reader slots and reuses scratch only up to 64 KiB. This is NOT a process
RSS limit: account state, validator/node projections, native/Ethereum receipts,
EVM trie and the 256-state historical RPC window still have separate costs.
Transaction/hash lookups without a secondary index can scan history; do not
claim constant-time historical hash lookup or globally bounded process RAM.

The active growing pair uses positional `ReadAt`, not mmap: a truncated mapped
inode can fault the process, whereas a positional read returns an error. Sealed
conversion generations retain the existing callback-scoped mmap reader. Neither
API permits retaining a borrowed byte slice beyond its callback. Decoding owned
Go objects is not zero-copy and does not eliminate allocations.

## Durability and checkpoints

Finalized blocks sync data before appending/syncing the index. History publication
occurs afterward under the chain lock. Durable write errors prevent another write
until verified reopening; an unindexed tail is rejected, never silently repaired.
In-place `Rewrite` is rejected for durable history. Offline conversion still
writes a separate generation and does not replace active files.

Checkpoint format 2 also seals the confirmed node-registration projection, so a
warm recovery need not decode the complete history just to reconstruct identities.
Old local checkpoints fall back to full verification once and are replaced by new
eligible checkpoints. This is a local cache format change, not a chain migration.
The original chain and anti-double-signing journals remain authoritative.

## Acceptance and rollout

Run `go test ./...` and `go test -race ./ledger ./blockchain ./internal/testnet`.
`TestIndexedHistoryFourValidatorsAndObserver` exercises four reopened validators,
a binary-format observer, subsequent finality, RPC equivalence, journal prefixes,
caller ownership and another reopen. These are local fixtures, not VPS evidence.
`TestIndexedHistoryAncestorBoundary` checks the 256-block hash window. Live-reader
tests cover concurrent reads/appends/close, cache bounds and malformed pairs.

Use `go test ./blockchain -run '^$' -bench BenchmarkIndexedHistoryRead -benchmem`
for reader allocations and latency. Do not infer Joules from these counters.

No service, active ledger, mining configuration, economic allocation or website
is modified by this source change. Existing binary-only installer remains a
candidate installer; it does not perform active generation cutover or mainnet
provisioning. Network acceptance, legacy freeze/import proof and separate mainnet
configuration remain deployment gates. Node-installer v2.1.6 transport/onboarding
acceptance does not supply those economic or HVM consensus proofs.
