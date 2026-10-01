# HVM dat/idx convergence candidate — not an activation release

The new hosts' five-second observations show ~0.0416% steal, unlike the older
v2 windows. Both .151 AND .152 have Docker listeners on 80/443. No HashBurst/IPFS
units appeared in their audits. This does not establish empty disks or sustained
capacity under load. No Docker, HB-Files or IPFS configuration is changed here.

## Format decision

Keep masterData.hbx only for legacy readers and historical economic migration.
HVM already uses blockchain.dat and blockchain.idx. Its index is already 20-byte
big-endian [BlockIndex uint64][FileOffset uint64][BlockSize uint32]. Struct memory
alignment is irrelevant: encode each field explicitly. Size includes the frame.

The old payload is uint32-BE length + gob. The manual-codec candidate is an HBX2
CRC frame containing HBB2 Block bytes. SAME FILENAMES DO NOT MAKE THEM COMPATIBLE.
The new binary pair lives in a separate immutable generation with FORMAT.json.
Never replace current runtime files with it before runtime integration.

ledger.OpenPair supports explicit GobPayload or BinaryPayload; ValidateLayout
checks index contiguity and bounds; WithPayload reads lazily. Mmap misses lend
encoded bytes only in a callback. Cache hits copy into owned scratch. Cache
budget bounds encoded cache bytes, NOT node RSS, decoded blocks or OS page cache.
Eight concurrent reads bound scratch work. A mapped inode must not be truncated
or overwritten. Live append/remap and consumers of Blockchain.Blocks remain open
integration gates. This patch does not silently claim to replace those consumers.

## Conversion tool

Build from this candidate in hvm-network:

    go build -trimpath -o hvm-ledger-convert ./cmd/hvm-ledger-convert

Use ONLY an existing offline, pinned HVM state or a properly isolated offline
copy with configuration/identity consistency. The command does not stop nodes:

    ./hvm-ledger-convert --config /absolute/existing/node.json \
      --expected-chain-id 4735490 \
      --expected-config-digest REVIEWED_CONFIG_DIGEST \
      --output /absolute/private-parent/new-generation

The expected digest is the reviewed shared config pin from the network plan,
not an invented value. The actual runtime Prepare path exclusively locks state,
checks identity and configuration pin, verifies versioned consensus and fully
replays projections, including existing APoW/BFT/EVM rules. No new ad-hoc
"SHA-512 plus timestamp" test replaces consensus. Administrative replay still
loads current history into RAM: the converter does NOT promise bounded source
validation memory or cheap initial replay.

After validation, each Block is encoded, round-tripped, written and read back
against the original encoded value. Original dat/idx, runtime pin and both
signing journals are SHA256-checked unchanged through conversion. No key,
configuration, HB-Files or IPFS file is rewritten. No masterData.hbx importer,
legacy freeze proof or genesis builder is supplied by this command.

WriteGeneration writes an unpublished sibling directory. It fsyncs data, then
index, then the format manifest and directory, renames the whole directory,
and fsyncs the parent. This publishes a PAIR as one namespace operation. It
never overwrites an existing generation. On prepublication failure its own
partial directory is removed; after rename failure of parent fsync, the result
may already exist and must be inspected. A stale conversion-lock file after a
crash is retained for manual inspection; do not unlink one for a live worker.
The parent must be trusted, canonical and not group/world writable.

This is a new offline generation, not a live append commit protocol. Optional
checkpoint.go remains a derived-state cache. Deferring durable finality or
signature journal writes to an epoch/checkpoint would violate crash recovery.
A future group-commit implementation must acknowledge finality only after its
required durable boundary; data Sync followed by index Sync alone is not atomic
visibility to concurrent readers without a committed-high-watermark protocol.

## Fleet procedure and limits

The known .153/.154/.155/.157 services are TESTNET validators (4735490).
64.31.4.9 is a non-signing observer. Five machines do not imply five votes.
Do not rename them mainnet or replace their testnet economic state.

`bash fleet-inspect.sh` collects current identities/roles/health sequentially.
It reads only, stops on error and does not certify common-height agreement.
No automatic cutover script is supplied while the node cannot open this codec.

After runtime integration and acceptance, the required rolling workflow is:

1. Pin and build/test the reviewed merge SHA once, ahead of maintenance. Preserve
   build metadata and hashes; do not fetch a floating master per node.
2. Check actual validator weights/quorum and common finalized commitment across
   the fleet. Keep the existing ingress available where possible.
3. Pause only the target's APoW miner, stop its node gracefully, verify exclusive
   runtime lock, then snapshot the quiescent dat/idx AND signing/recovery state.
   Keep private backups 0700, with local hashes; never print/copy private keys
   into reports. Live copying two changing files is not a coherent backup.
4. Run persistent offline migration with an immutable operation ID, verify roots,
   receipts, economic totals, config and identity/journal preservation. Preserve
   originals; no automatic deletion of hbx or legacy economic evidence.
5. Switch to the supported generation and pinned binary, validate the effective
   systemd ExecStart (including drop-ins), start exactly once, verify identity,
   common-height commitment and finality progress before the next node.
6. Only after all nodes agree, manually start/re-enable the selected miners and
   verify APoW proof, single reward of 50 HBT, recipient, supply and restart.

A target node has downtime during offline migration. Network continuity depends
on the actual quorum and healthy peers; zero downtime cannot be guaranteed from
machine count. Never roll back a signing journal after new signatures.

## Mainnet and HB-Files

Mainnet 4735489 remains separate; legacy 1337 and testnet 4735490 keep their
identities. The previously approved legacy freeze/import plus additional founder
billion needs actual freeze enforcement, authenticated snapshot, ownership
mapping, supply reconciliation and an approved validator/genesis manifest.
The output flag legacy_freeze_verified=false explicitly prevents presenting a
storage-format conversion as that proof.

Do not use hashburst-node.service for HVM on a host with the installer-owned
service of that name. Use hashburst-hvm-mainnet.service after real provisioning.
HVM RPC 18019 loopback and P2P 31317 are distinct from 8091 storage summary,
8093 mining aggregation and 8094 storage aggregation. Private IPFS key/bootstrap
membership must come from the existing federation. Do not regenerate its peer
identity or replace swarm.key while updating an HVM node.

Build normally with -trimpath and the reviewed Go toolchain. The historical
-gcflags=-dwarflocationlists=true flag is not a runtime performance optimization
or a way to obtain zero-overhead pprof. Profile only opt-in, separately from
energy measurements. Steady-state acceptance requires heap/RSS bounds measured
under specified history, state and RPC workload; no global RAM ceiling was
certified by PR #27.
