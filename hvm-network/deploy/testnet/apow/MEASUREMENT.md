# Four-miner calibration before APoW migration

Run `python3 measure-all.py` from the Mac. Only five SSH authentications are
needed; scripts run through those sessions and no binary is uploaded.
Requires the exact v0.4.0-rc.1 runtime and miner staged on the five nodes.

Each validator runs six synthetic jobs (14, 16, 18, 18, 18, 18 leading zero bits)
with its existing miner, its separate miner key and CPUQuota=25%, Nice=15.
Each job is a randomly named transient systemd service running as the miner user;
network access is restricted to loopback and RuntimeMaxSec=20 bounds its lifetime.
Only that measurement unit is stopped for cleanup. No node unit is stopped,
restarted, disabled or changed. The observer is checked but runs no measurement.
The permanent miner service remains stopped. No key is exported or generated.

The synthetic endpoint is a random loopback port/path, unrelated to /apow on the
node. Challenges use height 1 and a random parent, not a current chain job.
The real Go miner hashes, signs and self-verifies the proof; the collector checks
canonical digest, target and bound public identity. It does not independently
verify ECDSA. Signature/data are public; synthetic samples confer no reward.
Reports include successful nonce counts and GET-to-POST wall time, with explicit
timeouts. Short successful samples are not a mining fairness or liveness proof.
No claim is made that target_seconds is an enforced fixed block interval.

Five-node finalized commitment agreement is checked before and after measurement.
Configuration/pin bytes and signing journal prefixes are preserved. The report
contains a conservative parameter proposal; its activation_height is null and
it is not accepted as a migration configuration. Existing EVM gas policy remains
unchanged. Mainnet parameters must be approved and tested separately.

## Coordinated migration constraints

After calibration, recheck all five identities, live state and staged hashes.
Fix common parameters and a future activation height from an agreed snapshot;
retain the old protocol, all funds, identity fields and historical EVM rules.
The five nodes must be coordinated so that none crosses activation on old rules.
Before running --migrate-apow, stop the node processes and acquire their native
exclusive runtime locks. Preserve existing recovery snapshots and journal files.
A migration intent must be durable and a remote job must survive SSH disconnects.
Poll that job by identity rather than blindly replaying the migration command.
If any migration fails, retain state; never roll back signing journals.
All five candidates must pass offline validation before the validators resume.
Verify the actual executable paths and systemd sandbox execution permissions.
Start the separate miners and require fixed-height agreement, post-activation
work validity, one 50 HBT reward per finalized block, beneficiary binding, and
balance reconciliation separating rewards from fees and other transfers.
Then restart exactly one validator, verify journal prefixes/recovery and resumed
agreement, and repeat EVM/MetaMask acceptance. Staging/calibration do not certify
these live gates and do not activate mainnet.

The migration coordinator and live reward acceptance package follow measured
results. Do not hand-edit node.json or run migrations independently per VPS.
