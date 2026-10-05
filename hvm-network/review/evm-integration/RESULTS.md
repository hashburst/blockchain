# EVM integration validation - 2026-09-26

Source validation only. No VPS was upgraded and no production activation was selected.

Passed:
- Complete hvm-network Go test suite (uncached).
- Full blockchain race suite; targeted EVM race suite after final config isolation change.
- 15 execution-module tests under the race detector.
- go vet for node and execution modules.
- JavaScript syntax check for the manual MetaMask harness; not a wallet execution.

The integration test starts four localhost libp2p peers with actual HashBurst
mempools and chain projections. Signed Ethereum envelopes propagate to all four.
Each node independently validates and signs the proposal; the quorum certificate
finalizes a HashBurst block. The test covers transfer with fractional wei, gas,
contract deployment, storage and logs, finalized HTTP/WS queries, native spending
from the same account, disk reopen, preserved journal prefix and subsequent
precommit, and a non-signing observer replay. It does not replace a four-VPS
runtime/process-restart acceptance run.

Reproduce with `bash hvm-network/deploy/testnet/evm/verify-local.sh` using Go 1.25.7.

## Node suite

```text
?   	hashburst	[no test files]
ok  	hashburst/blockchain	2.855s
?   	hashburst/cmd/hashburst-testnet	[no test files]
?   	hashburst/cmd/hashburst-testnet-bootstrap	[no test files]
ok  	hashburst/cmd/hvm-native-canary	0.010s
ok  	hashburst/consensus	0.019s
?   	hashburst/deploy/testnet/observer	[no test files]
?   	hashburst/devnet/hvm-network/cmd/ctl	[no test files]
?   	hashburst/devnet/hvm-network/cmd/fixture	[no test files]
ok  	hashburst/devnet/hvm-network/cmd/node	0.066s
?   	hashburst/devnet/hvm-network/internal/devnet	[no test files]
ok  	hashburst/hvm	0.012s
ok  	hashburst/internal/bootstrap	0.179s
ok  	hashburst/internal/testnet	0.122s
ok  	hashburst/protocolv2	0.009s
ok  	hashburst/wallet	0.078s
```

## Race results

```text
ok  	hashburst/blockchain	18.924s
ok  	hashburst/blockchain	3.226s
```

## Release blockers

Pinned configuration migration/future activation height, public gateway write/WS
controls, live testnet four-validator/observer agreement and restart, actual
MetaMask evidence. Mainnet has a separate configuration and remains unactivated.
Legacy 1337 unchanged; active native testnet 4735490; reserved mainnet 4735489.
