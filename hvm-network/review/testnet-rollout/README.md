# Persistent testnet rollout — 2026-09-26

Evidence source: operator-supplied command output, not direct access to the VPS.
Runtime commit: 06d7c3bd2cff59b2e95b25ed09fe8509dd6de01a.
Testnet chain ID: 4735490. Legacy 1337 unchanged. Mainnet 4735489 not activated.
Shared configuration digest: 502c051c2cf6040b988fe2bc94ae984ab7f709e059918237110af678266ea215.

Four validators: hvm-testnet-v1 through v4, hosts .153/.154/.155/.157.
All 12 directed TCP links were reported reachable before activation.
Finality progressed on all four; the restart run ended at finalized height 28204.
Only v4 was restarted. Its journal prefix and identity pin were preserved;
a new PRECOMMIT above the pre-restart signing height 27983 was verified.
Later v4 health reported finalized height 28266, three peers, NRestarts=0,
fresh reactor snapshot, evidence_count=0, delivery_dropped=0.

Restart marker reported:
- journal prefix size: 29124560 bytes
- SHA256: fb3b1abdb6b662ee8520e195b8a05d24a13c2dbdea869ab2317ee52f1a9bd197
- pin SHA256: 2150978f294a5ff548ff3d9e3a100a0e1a5d9bfe3c0871232afad844ec62ec8b

The detailed Mac report directory is resume-results-avgmjaic; its raw JSON files
have not been imported. These are reported observations, not a signed attestation.

## Corrections consolidated

Allow AF_NETLINK for libp2p interface enumeration under systemd.
Use dedicated SSH sessions without multiplexing.
TryStatus=false means a busy mutex, not stopped consensus. Resume checks actual
finalized-height progress, identity/configuration and journal health.
Resume never re-promotes nodes and only permits a restart of v4.
The successful recovery test must not be repeated automatically.

## Remaining limits

Comparing next-validator roots across independently sampled moving heights is
not a fixed-height chain consistency proof. The resume gate can wait unnecessarily.
Older v1/v2 snapshots reported delivery drops; investigate growth rates during soak.
Startup readiness must tolerate replay time as persistent state grows.
Public RPC, application canary, WebSocket support and mainnet are not certified.
No production genesis, legacy chain state, private key or signing journal is changed.
