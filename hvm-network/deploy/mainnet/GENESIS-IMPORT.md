# Mainnet economic genesis candidate

This change integrates the approved import into Go state, genesis verification,
full replay, cloning and authenticated local recovery checkpoints. It is not a
mainnet activation, production checkpoint distribution or a new legacy rollout.

## Commitment and units

`ProtocolV2Config.genesis_import` must equal `genesis-import.json` exactly.
The source-bound nullifier is SHA256 of
`HashBurst/legacy-import/v1:1337:10:<approved terminal hash>`.
It does not depend on an evidence filename or a mutable report identifier.

The transition is package-private, only accepts an empty native state and
publishes balances plus the consumed receipt under one state lock. There is no
import transaction or RPC method. The native root uses
`HASHBURST_HBT_STATE_V3_IMPORTS` when a receipt exists. The receipt commits the
source, target chain, recipient, import amount, separate founder allocation and
accepted freeze report hash. Removing or modifying it changes the native root.
Old states without imports keep the exact V2 root encoding; nil genesis_import
is omitted from protocol JSON, preserving testnet configuration digests.

Mainnet genesis commits this root and a hash of the full protocol configuration.
Replay reconstructs the allocation once from the configured genesis. A cache
restores balances and receipts together and checks their root against the block.
Corrupted checkpoint data causes authenticated replay fallback, not a reimport.

| Allocation | Native units (8 decimals) | HBT |
| --- | ---: | ---: |
| Existing legacy value | 45000000000 | 450 |
| Separately approved founder allocation | 100000000000000000 | 1000000000 |
| Initial total | 100000045000000000 | 1000000450 |
| Additional issuance from migration itself | 0 | 0 |

The founder allocation is new initial issuance distinct from the conserved
legacy import. The initial total is not a claim about later mining rewards.

## Offline command

Build from the reviewed commit:

```sh
go build -o hvm-mainnet-bootstrap ./cmd/hvm-mainnet-bootstrap
```

Required inputs are a complete reviewed `ProtocolV2Config` JSON with the
`genesis_import` object, the unchanged accepted freeze report, and the offline
11-block terminal generation. The command does not supply provisional protocol
values or silently create validator identities.

```sh
./hvm-mainnet-bootstrap \
  --protocol /absolute/private/mainnet-protocol.json \
  --freeze-report /absolute/private/FLEET-FREEZE-HA-1791220850167915000.json \
  --source /absolute/private/terminal-generation \
  --out /absolute/private/new-economic-checkpoint
```

These are input-path placeholders, not a VPS deployment command. The output
parent must exist, be canonical and not group/world writable. Existing output
is rejected. Under an exclusively controlled parent, a cooperative lock and
sibling staging directory prevent concurrent writers and partial publication.
Data, index, metadata and directories are synced before/after directory rename.
On failure no live node is repaired, reset or restarted.

The output contains `ledger/blockchain.dat`, `ledger/blockchain.idx`, protocol
JSON and `CHECKPOINT.json`. Genesis is reopened through the actual HVM reader
and reconciled before publication. The manifest always states
`activation_allowed=false` and `production_import_executed=false`.

## Acceptance and remaining inputs

Local Go tests cover concurrent single-use import, invalid/cross-chain rejection,
clone/replace preservation, genesis reopen, full replay, actual finalized EVM
checkpoint recovery, corrupt-cache fallback, independent native-root restoration,
output overwrite refusal and a fabricated freeze report. The checkpoint test
uses four synthetic validator identities and development parameters, not the
production validator manifest. Full Go suite and targeted race tests passed.

Still required before production activation:

- Fresh signed mainnet validator public registrations, tied to the intended
  nodes and real TEP identities; private keys stay on their owning hosts.
- Approved protocol timing, gas, APoW/difficulty/emission parameters and funding
  of validator bonds/fees. The testnet bootstrap's reward-generating setup must
  not be used as a zero-extra-issuance mainnet registration procedure.
- Production validator checkpoint, supply reconciliation, runtime/restart and
  wallet/finality acceptance, bound to the same source and complete manifest.

The mainnet runtime refuses an economic genesis-only checkpoint and a profile
without the approved import. This gate is not replaced by editing a JSON flag.
The existing managed-fleet freeze report remains the starting evidence; no
legacy reinstall or repeat rollout is required. Final release and website
publication remain pending the production acceptance above.
