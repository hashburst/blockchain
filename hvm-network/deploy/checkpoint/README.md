# HVM recovery optimization

This package updates only the runtime on the existing APoW-migrated testnet (4735490).
It does not migrate configuration, enable miners, provision mainnet, or change legacy 1337.
Do not treat a successful build as live acceptance.

## Install one node at a time

Copy the Linux amd64 archive to the selected VPS, extract it, then:

```
sha256sum -c SHA256SUMS &&
python3 upgrade-node.py install --node hvm-testnet-v3 &&
python3 upgrade-node.py verify --node hvm-testnet-v3 --timeout 7200
```

After an SSH or readiness timeout repeat only `verify`. Do not repeat `install`,
restart the service, delete cache files or modify `node.json` to shorten a wait.
The first start still validates the existing chain and creates a local checkpoint.

Require `CHECKPOINT_RUNTIME_RECOVERY_AND_JOURNAL_PREFIX_OK` before updating the
next node. The order is v3, v1, v2, v4, then hvm-testnet-ingress. v3 currently runs
the preceding lock fix; this optimization is a separate candidate.

Each installation verifies the exact predecessor binary, actual process and effective
systemd ExecStart, and backs up drop-ins. Identity pin, configuration and prefixes of
both signing journals are checked before and after startup. The late override adds
the new executable without disabling the service's sandbox protections.

## Acceptance before miners

Use the original APoW activation-plan.json and the matching runtime-release.json
with the APoW coordinator. Never edit the original plan to substitute binary hashes.
Require five-node identity/configuration agreement and finalized commitments at a
common height. Only then execute the explicit operator miner-start step.

Verify APoW work, its beneficiary and exactly 50 HBT per eligible finalized block,
then restart a single validator and prove journal preservation and resumed finality.
Repeat EVM/MetaMask transfer, deployment, call, receipt, log and WebSocket acceptance.
Mainnet remains a separate deployment and economic configuration.

## Recovery measurements

Logs distinguish `HVM_CHECKPOINT_FALLBACK`, `HVM_CHECKPOINT_SAVED`,
`HVM_CHECKPOINT_RESTORED`, chain verification and replay completion. A healthy
service while replaying may not yet listen on port 18009. Do not open a public
firewall port to solve a localhost startup refusal.

Record cold and warm startup wall time, systemd CPUUsageNSec deltas, peak RSS and
bytes read on the same VPS and same finalized history. The initial upgrade is cold.
Warm acceptance requires a subsequent controlled restart, after the other validators
are healthy. Retain the journal-prefix and fixed-height commitment checks.

A direct electricity-saving percentage cannot be inferred from CPU time alone;
energy measurements require host telemetry and a stated measurement boundary.

## Checkpoint custody

`recovery-checkpoint.key` is a random 32-byte local cache-sealing secret, mode 0600.
It is not a coin wallet, seed, validator key or mining identity. Do not upload it with
logs, publish it, or copy it to new nodes. The authenticated cache also binds the data
path, runtime identity pin, protocol configuration, finalized block and exact stored
chain prefix. Signing journals and recovery locks are always loaded independently.

Missing keys/caches, incompatible configuration or detected corruption cause full
verification and replay. `HVM_FULL_REPLAY=1` disables checkpoint use/writes for an
independent audit. Set that only in a planned maintenance operation; do not start a
second process against live validator storage.

New nodes, including nodes connected over TEP, perform full bootstrap verification.
This feature does not establish trust in peer-supplied state snapshots or make TEP
transport a substitute for consensus authentication.
