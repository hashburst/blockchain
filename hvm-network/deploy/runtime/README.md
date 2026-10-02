# Runtime integration candidate

The node now reads either legacy Gob frames or HBX2/HBB2 frames, validates the
same 20-byte index, executes its existing consensus/state replay and appends in
the detected format. Mixed formats and corrupt binary CRCs fail closed. Fresh
legacy directories retain Gob; no implicit conversion occurs.

Periodic runtime recovery checkpoints freeze projections into bounded encoded
bytes under the existing chain lock. One writer compresses and durably publishes
outside that lock, with at most one active and one pending payload. Startup/audit
checkpoint writes remain synchronous. State.Close drains the writer before
releasing the process lock. Snapshot preparation/EVM trie export still cost CPU
and allocation; this does not promise zero finalization latency or lower Joules.
Blocks and signing journals retain synchronous durability independently.

## Single-file installer candidate

hashburst-install.py runs on an existing TESTNET VPS. It updates a runtime binary
only, not the ledger format. It does not provision mainnet, change node.json,
create keys, start miners, alter IPFS, or automatically roll back signing state.

Inputs: tested Linux binary and reviewed release JSON with chain_id=4735490,
source_commit (40 hex), sha256, predecessors (approved prior binary SHA256 list).
Hash verification proves correspondence to that reviewed manifest, not provenance
if both binary and manifest came from an untrusted party. No install manifest is
published for this development candidate yet.

    python3 hashburst-install.py install --node hvm-testnet-v3 \
      --release release.json --binary hashburst-testnet
    python3 hashburst-install.py status --node hvm-testnet-v3 \
      --release release.json --binary hashburst-testnet
    python3 hashburst-install.py verify --node hvm-testnet-v3 \
      --release release.json --binary hashburst-testnet --timeout 7200

Install copies immutable job inputs and starts a systemd oneshot independent of
SSH. Its phase record is replaced atomically and fsynced. resume resubmits a
finished/failed job but follows its recorded phase: prepared -> stopped ->
configured -> start_requested -> verified. No second start is issued once the
start intent exists. A crash between start-intent persistence and systemctl start
requires inspection; verify cannot invent success or retry an uncertain start.
A reboot interrupts a transient systemd job; resume is required after inspection.

The candidate requires a reviewed quorum/readiness check and target-miner pause
before install. It is NOT yet an automatic fleet coordinator. Upgrade one node,
verify finality progress and common-height agreement against unchanged peers,
then proceed to the next target. Four validators plus one observer is not five
votes. A testnet paused at the APoW activation barrier does not satisfy this
installer's finality-progress check automatically.

Tests cover source format replay/append/corruption, immutable async snapshots,
race detection, manifest network exclusion, evidence-prefix preservation and
command matching. Live systemd failure-injection/fleet acceptance remains open.

## Gates still open

The current runtime still retains Blockchain.Blocks. This commit does not claim
a bounded total node heap, mmap use on the live append file, authenticated peer
bootstrap or a full lazy-history runtime. A production generation switch also
needs complete state/identity/journal handling, rather than replacing two files
individually. Mainnet legacy spending freeze, economic snapshot reconciliation,
validator/genesis manifest and live acceptance remain separate gates.
