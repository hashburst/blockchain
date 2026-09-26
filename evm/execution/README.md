# HVM Network Ethereum execution

Development branch, not activated on the deployed testnet or mainnet.

The HashBurst node now imports this module. `hvm-network/blockchain/evm_*.go`
connects signed Ethereum envelopes to the real mempool, libp2p gossip, BFT
proposal validation, native account state, durable HashBurst blocks and replay.
There is no independently advancing Ethereum chain or freely allocated balance.

## Execution and consensus

- Pinned go-ethereum 1.17.6, explicit Cancun rules. Protected legacy, access-list
  and dynamic-fee envelopes; wrong-chain, blob and EIP-7702 envelopes rejected.
- An optional consensus `evm` configuration specifies activation height, gas
  limit and fixed base fee. Nil preserves existing behavior and config digest.
  This initial base-fee policy is fixed, not Ethereum's variable EIP-1559 policy.
- Version 3 blocks commit original signed bytes, EVM state/receipt roots and gas
  usage in the same HashBurst hash that validators sign. Old hashes are unchanged.
- Native effects execute first; Ethereum effects execute second. One account
  balance/nonce is projected in native units and wei. Fractional wei survives
  native transactions, finalization and replay; no floating-point conversion.
- Consensus commit writes the existing block store before changing projections.
  Restart replays those blocks and checks commitments. Signing journals are not
  reset or migrated by this implementation.
- RPC admission reserves nonce/funds without applying canonical effects.
  Competing native/Ethereum reservations are rejected; consumed nonces are
  pruned after finality. Proposal selection does not replay each pending prefix.

## Node APIs

When explicitly configured, the loopback runtime registers `/evm` and `/evm/ws`.
The existing public gateway has NOT been changed to expose these paths.

Implemented: chain ID, block number, balance, nonce, code, storage, receipt,
signed raw admission, call, gas estimation, block by number/hash, transaction by
hash, gas price, priority fee, fee history, bounded historical log queries,
net_version, client version, newHeads/logs subscriptions and unsubscribe.

Subscription hashes identify actual finalized HashBurst blocks. Ethereum header
fields are a projection, not an independently hashed Ethereum consensus chain.
Queries support latest/finalized/safe and pending simulation. Arbitrary historical
account-state queries and state overrides are explicitly unsupported. Historical
receipts/logs are replayed from canonical blocks. Slow subscriptions are detached
with bounded queues; public socket-lifecycle and gateway limits still need review.

## Reproducible tests

Go 1.25.7:

```sh
(cd evm/execution && go test -count=1 ./...)
(cd hvm-network && go test -count=1 ./...)
(cd hvm-network && go test -race -count=1 ./blockchain -run '^TestEVM')
```

The integrated test uses four actual HashBurst chain instances, localhost libp2p
gossip, recovered Ethereum signatures, four-validator finalization, native/EVM
accounting, contract deploy/storage/logs, actual HTTP and WebSocket clients,
canonical block/receipt queries, fee history, disk reopen and subsequent signing,
and an observer replay with no signatures. This is local automation, not proof
that the production VPS have received or activated the code.

`persistence.go` remains a separately tested immutable replay-store utility.
The integrated node deliberately uses its own canonical block store instead;
it does not create a competing execution database.

## Remaining release gates

1. Review the new consensus format/economics and adversarial resource limits.
2. Implement and test an explicit migration of the pinned runtime configuration;
   choose a future activation height only after all validators/observer are ready.
   Do not edit/delete runtime.pin or journals to bypass the existing pin check.
3. Upgrade public gateway filters/write and WebSocket controls; make read APIs
   consistently expose the activated Ethereum projection.
4. Roll out testnet, compare all four validators and observer at a common finalized
   height, restart one validator and verify retained journal/new signatures.
5. Run the supplied manual MetaMask canary with a funded testnet account and retain
   actual wallet evidence. Then prepare mainnet configuration separately.

Testnet 4735490; mainnet 4735489 reserved; legacy 1337 unchanged. No test here
certifies complete MetaMask interoperability or mainnet readiness.

Dependency: go-ethereum library LGPL-3.0; binary distribution must include its
applicable notices/compliance materials. This PR supplies source, not a rollout
binary. Upstream: https://github.com/ethereum/go-ethereum/tree/v1.17.6
