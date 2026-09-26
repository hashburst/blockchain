# HVM Network Ethereum execution adapter

Status: development only, not imported by the running HVM Network runtime.
No endpoint, ledger migration or network activation is enabled by this module.

The adapter pins go-ethereum 1.17.6 and explicitly freezes Cancun execution
rules. It validates protected legacy, access-list and EIP-1559 signed transaction
envelopes and applies a candidate block to a copy of the supplied state. Invalid
transactions reject the candidate. EVM revert consumes gas and nonce but reverts
contract effects. Geth generates receipts, logs and receipt/state roots.

The subscription adapter implements newHeads, filtered logs and unsubscribe;
tests use an actual localhost WebSocket connection. The adapter has bounded
queues and a subscription cap. It is not yet registered in the public gateway.
Slow consumers are detached; production integration must close their sockets
and support historical catch-up, authentication/rate limits and idle timeouts.

Run with Go 1.25.7:

```sh
cd evm/execution
go test -count=1 -v ./...
```

## Integration gates still required

1. Versioned activation in HashBurst consensus, including EVM transaction bytes,
   state/receipt roots, block gas limit and base-fee transition validation.
2. Connect the durable replay store to finalized HashBurst blocks and validate
   its activation anchor against the agreed native checkpoint. The store is
   implemented and tested in isolation; no running node uses it yet.
3. Specify and test conservation between native 8-decimal HBT and 18-decimal
   EVM balances. No silent rounding, duplicate balances or unrestricted minting.
4. Admission, gossip, nonce reservations and block proposal integration for
   Ethereum envelopes. No conversion to unsigned native transactions.
5. Ethereum RPC block/transaction/receipt/log mappings, eth_call, estimateGas,
   fee history, public write limits, and bounded WebSocket subscriptions.
6. Four-validator testnet agreement, observer agreement, restart/replay and real
   MetaMask transfer/deploy/call/event tests, followed by separate mainnet config.

Chain IDs: testnet 4735490; mainnet 4735489 reserved; legacy 1337 unchanged.
Support in this adapter does not activate either network. Blob and EIP-7702
transactions are rejected, rather than being partially interpreted.

References:
- https://geth.ethereum.org/docs/developers/geth-as-a-library
- https://ethereum.org/developers/docs/apis/json-rpc/
- https://github.com/ethereum/go-ethereum/tree/v1.17.6

Dependency licensing: go-ethereum library is LGPL-3.0; distribution of linked
runtime binaries must include the applicable notices and compliance materials.
This change distributes source and module references only.

## Durable execution projection

`OpenStore` pins a chain/activation anchor, balances and the previous BLOCKHASH
window. It requires a private directory and exclusive process lock. `Append`
re-executes the signed transactions and checks state root, receipts root and gas
against the supplied finalized commitments. It publishes immutable per-height
records with file and directory fsync before advancing memory. An ambiguous I/O
failure poisons the handle until reopen. Reopen replays all records and rejects
corruption, gaps, changed anchors and wrong commitments. It never opens or
rewrites the existing native blockchain database or signing journal.

The activation allocation is an input, not a faucet: consensus must derive it
from the approved native snapshot and prevent double accounting before calling
this API. `NativeToWei` and `SplitWei` perform exact conversion and preserve dust;
they do not themselves implement the cross-ledger settlement or migration.

Tests cover transfer replay, deployed code/storage replay, duplicate writers,
changed anchors, corrupt records, missing ancestor hashes and conversion overflow.
The record format and storage backend remain developmental: full-history replay,
checkpoint acceleration and recovery at the actual consensus commit boundary
still require node integration and distributed validation.
