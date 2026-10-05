# Managed legacy archive rollout

`rollout-archive.py` installs **one host per invocation**, in this order:
64.31.4.9, 77.90.188.157, 77.90.188.155, 77.90.188.154, 77.90.188.153.
It does not import mainnet, sign another terminal block, or modify the HVM
validator/observer units, HB-Files, IPFS, transport identities or wallets.

The source is the already signed and restart-tested generation under
`/root/hashburst-terminal-74338c75/generation` on 64.31.4.9. Data/index and
terminal/state commitments are pinned in the installer. The archive binary
is copied from the same directory's parent, as tested previously; its SHA256
is recorded and checked against the running executable. No new Go build is
necessary. The original data files under `/var/lib/hashburst` remain intact.

On the administration Mac, from a pinned checkout of this revision:

```sh
python3 hvm-network/deploy/legacy/rollout-archive.py \
  --deployer-root /absolute/path/to/HashBurst_Deployer \
  --host 64.31.4.9
```

SSH/SCP ask for credentials through the terminal. The tool does not store
passwords. It downloads only the public terminal pair and archive executable.
The local `legacy-archive-rollout-state.json` prevents changing host or
installer/binary revision during an unfinished rollout. Keep the same
`--deployer-root` and checkout for every invocation.

## Interruption and acceptance

Repeat the **same command** after a disconnection. A content-addressed remote
stage and non-blocking systemd job retain progress. Uploads use temporary
names, and a rerun accepts only matching completed artifacts. An interrupted
worker can resume after masking the old unit or publishing the archive unit.
A verified worker is not rerun: the next invocation performs fresh read-only
verification instead. Never edit `completed`, clear `in_flight`, or restart
the old writable service to bypass a failed check.

The worker saves the old unit, disabled overrides and old ledger pair under
its root-only staging backup. It stops and disables only
`hashburst-node.service`, durably masks it, then installs a hardened unit of
the **same name** running the archive as a dynamic unprivileged user. It checks
stop dependencies first. There is intentionally no automatic rollback to a
writable legacy runtime. A failed migration may leave this legacy API offline;
inspect the retained worker journal and resume the archive installation.
The archive binds localhost:8009. Existing nginx proxying to that address is
preserved. Public direct access to port 8009 is intentionally removed.

Each successful report checks the effective unit, running executable digest,
terminal pair, health, eleven blocks, 450 HBT balance, rejected POST routes,
absence of processes named hashburst-node, and unchanged state/PID of observed
HVM/HashBurst/IPFS/TEP services. This is evidence about the managed service,
not proof that no copy of a private key or ledger exists elsewhere.

`ONE_LEGACY_ARCHIVE_VERIFIED=<host>` authorizes the next invocation for the
next host. After all five succeed, run `rollout-archive.py --deployer-root /same/path --audit-fleet` for a **fresh fleet-wide audit**, plus
restart verification of the archive service, before approving mainnet import.
Individual reports deliberately retain `fleet_freeze_verified=false` and
`mainnet_import_executed=false`. A current all-host freeze attestation and
one-time import reconciliation remain separate gates.

## API and testing scope

The tested archive serves health, blocks, transactions and balances; it is not
a replacement for live mining, node registration or discovery. Other legacy
API routes can return 404 and mutation methods return 405. Dashboard consumers
must display archived status rather than claim live legacy activity.

Python regression tests inject interruption immediately after the durable
mask and after archive unit publication. They verify resumption, preservation
of original files, disabling of overrides, digest refusal before service
mutation and the one-host gate. Systemd is mocked in these tests; the first
real host is the systemd canary. No remote deployment is claimed by the tests.

The fleet audit sets `fleet_freeze_verified=true` only for these five managed
legacy services at the reported observation times. It excludes unmanaged
copies and keeps `activation_allowed=false`; it is not a mainnet activation.
