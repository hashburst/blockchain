# Implementation test evidence

Base: db5d103c68ed6ebeb4db9938afa9f86e7c2deb87 (PR22 foundation).
Go 1.25.7, Linux amd64. Logs were produced from this change set.

- Entire hvm-network module: PASS.
- APoW tests with race detector: PASS.
- Entire evm/execution module, including strict Solidity artifacts: PASS (see evm/strict/TEST_RESULTS.txt).
- Four blockchain instances finalize work-bearing blocks with actual signed QCs.
- Four reactors recover from lost queued messages and retained locks.
- Persistent reopen preserves signing journal bytes and the finalized block hash.
- Native work ingress rejects a non-loopback client.

These are isolated tests, not VPS deployment results. Public configuration, keys,
genesis, balances, signing journals and chain IDs have not been changed.
No mainnet readiness or complete ERC conformance is certified by these logs.
