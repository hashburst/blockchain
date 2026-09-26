# Four-validator API rollout — 2026-09-26

Evidence: operator-supplied terminal output from API Resume v1.1.2; no direct VPS observation by the repository maintainer assistant. Raw JSON reports have not been imported.

- v1 and v2: VERIFIED_NO_RESTART.
- v3 and v4: UPGRADE_RECOVERY_OK after replay/readiness waits.
- Four-validator commitment agreement at explicit finalized heights 39261 and 39274.
- HVM_NETWORK_TESTNET_API_ROLLOUT_COMPLETE.
- FOUR_VALIDATOR_JOURNAL_PREFIXES_VERIFIED.
- Report directory: api-completion-pdphp3vp.

Runtime source: f7811f75ca1c1a5269d4f4faed0ee0cdcb2bd516.
Runtime binary SHA256: 2166ef8177aec64d1ea191ee00f5abf9e76f4779c072284b94a91076a14e9532.
Wrapper archive SHA256: f6eed66a7edf548ae350ab7d916e184183cb15805c262780c192bbc91dbfe00b.

Read-only HTTP transport retries are bounded; identity/protocol errors remain fatal. New upgrades are limited to v3/v4. Existing upgrades are checked without restart. Per-action errors identify the node and are retained in the report directory.

The comparator compares commitments at one explicit common height, not independently sampled moving validator roots. It does not independently verify certificate signatures. The observed finality advanced by 13 blocks between comparisons.

Testnet chain ID 4735490; legacy 1337 unchanged; mainnet 4735489 not activated. No data or signing journal was reset. This records private validator API rollout completion only. Public observer provisioning, native HVM application canary, public ingress, full EVM/MetaMask support and mainnet acceptance remain pending.
