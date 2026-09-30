# Coordinated APoW testnet migration

This workflow targets testnet 4735490 only. It does not activate mainnet or
change legacy 1337. It uses the exact already-staged runtime and CPU miners.
Live reports remain local; do not commit them to the repository.

The measured initial profile is 12 bits, minimum 8, maximum 14, window 32,
and target 5 seconds. The EVM gas limit is unchanged. The activation height
is selected from fresh health responses, 3000 blocks beyond the highest head.
A plan must be executed promptly: preparation rejects a margin below 1501.

## Operator sequence

Run from the package on the Mac. Verify SHA256SUMS first.

1. `python3 coordinator.py plan --measurement PATH_TO_MEASUREMENT_REPORT.tar.gz`
2. `python3 coordinator.py migrate --plan activation-plan.json`
3. `python3 coordinator.py jobs --plan activation-plan.json`
4. `python3 coordinator.py start --plan activation-plan.json`
5. `python3 coordinator.py verify --plan activation-plan.json`
6. Explicit manual miner start: `python3 coordinator.py miners --plan activation-plan.json`
7. After at least 64 APoW blocks have finalized:
   `python3 coordinator.py audit --plan activation-plan.json`
8. `python3 coordinator.py restart --plan activation-plan.json`
9. `python3 coordinator.py restart-check --plan activation-plan.json`
10. Repeat `audit` and the existing public EVM/MetaMask acceptance checks.
11. `python3 coordinator.py closeout --plan activation-plan.json` restores the
    original automatic restart policy after checking the APoW and recovery gates.

Steps 2-4 are a planned testnet outage. Do not execute another installer or edit
node.json concurrently. The Go migration writes node.json and runtime.pin
together with its durable intent, preserving all other protocol settings.
Editing node.json separately is not a supported step.

Preparation validates all five live identities, configuration digests, pins,
binary checksums and staged executable permissions before any stop. A persistent
systemd drop-in blocks node startup until the all-node offline gate has passed.
The coordinator submits stop requests to all five nodes; after each has stopped,
it records hashes of its own chain files, recovery snapshot and signing journals.
Nodes need not have identical database bytes or signing snapshots.

Migration runs in five independent systemd oneshot jobs. They have no SSH lifetime
or client timeout dependency and no automatic retry. Each runs the existing Go
migration, a full offline check as the node's service account, and verifies exact
preservation of chain files and journals. All five must succeed before any
runtime is started. Only the verified runtime file is added to ExecPaths;
NoExecPaths and other service hardening are retained. RootDirectory/RootImage
remapping is rejected before stopping because it requires a deployment-specific
path mapping.

## Disconnects and failures

Closing SSH does not cancel submitted systemd jobs. Reconnect and run `jobs`.
A 4-hour polling deadline does not terminate a job. Repeat `jobs` to keep observing.
The plan and the local activation-plan-evidence directory must be retained.

If interrupted before all jobs are submitted, repeat `migrate` with the SAME
plan: recorded preparation and snapshots are checked, loaded jobs are retained,
and successful jobs are not rerun. A failed job is never automatically retried.
A launch intent without a systemd job fails closed for inspection. Do not make
a new plan after any node has stopped or changed configuration.

If interrupted during `start`, repeat `start` with the same plan. Previously
authorized nodes are not restarted. A start authorization whose service failed
requires inspection; this command deliberately does not repeatedly restart it.

Remote evidence is under `/var/lib/hashburst-apow-rollout/PLAN_SHA256/`.
The migration job is `hvm-apow-migrate-PLAN_SHA256_FIRST_16.service`.
Its `migration.log`, `FAILED.json`, `DONE.json` and `snapshot.json` diagnose the
specific failure. Never restore an old chain, journal or recovery snapshot.

The migration guard leaves Restart=no during acceptance to avoid unattended
replay/crash loops. Restoring the original restart policy is a separate closeout
step after live acceptance, by removing only 80-apow-migration-guard.conf and
reloading systemd. Keep 81-apow-runtime.conf. No restart is necessary for reload.

## Acceptance scope

The auditor reads bounded records from the existing block/index files with
read-only file descriptors. It independently checks the block hash, APoW
signature/target, proof binding and exactly one native system reward of 50 HBT
to the proof beneficiary. The local finalized commitment must match. The
coordinator compares all five nodes at each of 64 common heights. Certificate
presence and equality to the node's commitment are checked; this tool is not
an independent validator-set/QC signature audit or a full historical replay.
The node's offline check/replay remains responsible for consensus validation.

The restart test affects v4 only and is sent once. It preserves journal prefixes
and requires subsequent progress, identity preservation, and a newly recorded
precommit included in a finalized certificate. Use restart-check after a timeout;
never repeat restart blindly. Mainnet, ASIC/GPU support and general EVM compliance
are not certified by this testnet acceptance.
