# Local recovery checkpoints: design and publication gates

## Invariants

The canonical block store, genesis, protocol chain ID, difficulty, rewards, validator
selection and journal formats are unchanged. A checkpoint contains derived state;
it is not a new source of economic allocations. Lock and signing-journal validation
still runs on every start. No checkpoint contains signing keys.

The trust boundary is explicit: the checkpoint is authenticated with a node-local
secret created only by the validated runtime, outside the checkpoint payload. The
MAC covers its compressed serialization, including protocol digest, node/path/pin
binding and SHA-256 of the exact canonical file prefix. An attacker with the sealing
key and write access can forge this cache; protecting the key is required. This is
not an independently certified snapshot import protocol. Use full replay for an
independent audit or when local key integrity is in doubt.

## Work reduced

A cold start performs complete verification and execution. At a finalized height at
least 256 blocks behind head it stores the execution projection. Subsequent starts
validate the index and file frames, authenticate the cache, hash the canonical prefix,
restore the committed projections and fully verify/execute the suffix. Native/HVM and
validator roots are recalculated. EVM trie nodes and contract code are restored with
the original root. At least 256 suffix blocks reconstruct the retained historical RPC
window. Old receipts retain RPC metadata as well as their consensus fields.

The EVM checkpoint includes only reachable account/storage trie nodes and referenced
contract code. Gzip at BestSpeed is lossless and favors low encoding CPU cost. Separate
limits bound encoded and expanded data at 64 MiB. Oversized cache writes are skipped;
consensus persistence still completes. This limit bounds the serialized checkpoint,
not the total process RSS or all Go allocations during state capture.

The loader reads frames and index entries through bounded buffers in a single pass,
checks frame bounds before allocation and reuses its frame buffer. This removes the
separate index scan and many small reads. Blocks remain resident in the current
runtime: this change does not claim constant-memory chain storage or pruning.

After startup, every 1024 finalized blocks a checkpoint is written via a private
temporary file, fsync, atomic rename and directory fsync. Two slots retain an older
eligible checkpoint. Prefix digest work on subsequent writes is incremental, using
only in-process hash state. Every restart recomputes prefix integrity from disk.
Checkpoint generation is synchronous under the chain lock; measure its latency before
public activation. The block database and historical receipts are not pruned.

## Required gates

- Full replay and checkpoint replay produce identical native, HVM, EVM and validator roots.
- Solidity code/storage, receipt metadata and the retained historical API window agree.
- Missing key, wrong identity/configuration, changed prefix, truncated/corrupt cache,
  malformed index and expansion limits cannot lead to unchecked state acceptance.
- APoW, consensus locks, persistence failure and double-sign protection regressions pass.
- Four validators and observer agree at a common finalized height after rollout.
- One controlled warm restart preserves journal prefixes and resumes finality.
- Measure cold/warm elapsed time, CPU time, allocated bytes/peak RSS and disk reads.
- Testnet and mainnet publication each require these gates on their own configuration.

No universal speed factor, TPS figure, energy reduction or ASIC compatibility follows
from a checkpoint unit test. Existing initial synchronization is still full verification.

## Technical references

- Geth state synchronization and root-verified state: https://geth.ethereum.org/docs/fundamentals/sync-modes
- Geth v1.17.6 StateDB and trie APIs (the version pinned by this repository):
  https://github.com/ethereum/go-ethereum/tree/v1.17.6/core/state
- Gzip lossless stream implementation: https://pkg.go.dev/compress/gzip
- HMAC authentication: https://www.rfc-editor.org/rfc/rfc2104

These describe the primitives and reference architecture, not validation of this patch.
