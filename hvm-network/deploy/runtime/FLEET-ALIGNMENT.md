# Sequential corrected-reactor alignment

This candidate aligns testnet 4735490 only. It does not migrate ledger formats,
activate mainnet, change mining configuration, or publish releases/sites.

Prerequisite: successful `recover-v1.py` and its local
`v1-recovery-2719806647f2/ACCEPTANCE.json`, plus its cached checked binary.
Keep the old deployer available for SSH and GitHub helpers; do not run its deploy
or publish commands. The original deployment.json and pinned PR #27 remain unchanged.

Run from the administrator's deployer root:

```
caffeinate -i python3 /path/to/reviewed-checkout/hvm-network/deploy/runtime/align-fleet.py --deployer-root "$PWD"
```

The checked-out revision must pass CI before any server action. Each invocation
updates at most ONE runtime: observer, v4, v2, v3. v1 remains untouched.
Repeat exactly the same command to resume or advance. A monitor deadline retains
remote systemd jobs. Once start_requested is recorded, only verification is retried.
Do not erase state, restore signing journals, or restart a validator to fix a timeout.
Pre-start failures require explicit inspection. SSH may still require repeated
password entry. FLEET_PHASE is printed after each short remote polling call.

Before stopping a runtime all five nodes must agree on commitments at a common
finalized height and show progress. Three validators plus an observer are NOT four
votes; the observer never counts toward quorum. After each replacement the same
five-node gate must pass. Digests, identities and pin checks survive the update;
installer checks preserve signing-journal prefixes. Proof comparison relies on node
consensus validation and is not an independent QC signature audit.

Only the known 3a77... predecessor is accepted. The runtime is the already tested
30a0... binary from commit 271980..., not a rebuilt or silently replaced binary.
The new installer replaces only an exact known predecessor override, archives it,
fsyncs and atomically renames the replacement. Arbitrary overrides are refused.
Old v1 recovery scripts/manifests remain byte-for-byte unchanged.

Mining is not stopped or started by this command. It must remain separately
supervised; a lack of finalized progress blocks advancement. Network continuity
is checked, not promised when a validator or the only available miner is unavailable.

Completion marker: FIVE_CORRECTED_RUNTIMES_AGREEMENT_AND_PROGRESS_OK.
Evidence: fleet-alignment-2719806647f2/ACCEPTANCE.json. This proves sequential
runtime alignment and observed progress, not mainnet economic migration or reward
correctness. Follow with APoW issuance/receipt/supply reconciliation and checkpoint
restart acceptance. Release requires merging the stacked PRs and testing builds on
all supported targets. Mainnet additionally needs approved separate genesis,
identities, allocation reconciliation and enforceable legacy spend-freeze evidence.

Soup-inspired prefetch is a separate optimization candidate; do not change consensus
or the pinned binary during this rollout. Compare cold replay, checkpoint replay and
bounded prefetch using CPU time, RSS, allocations and identical state roots before
claiming a speed or energy benefit.
