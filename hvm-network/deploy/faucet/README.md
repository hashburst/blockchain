# HVM faucet runtime

This directory implements the approved faucet policy without placing a signing key
in the public API process.

## Process boundary

`hvm-faucet` owns policy, idempotency, cooldown/caps/reserve accounting, the
append-only audit and native HVM RPC submission. It has no keystore or password.

`hvm-faucet-signer` is a separate loopback-only process. It owns the encrypted
keystore/password boundary and independently refuses the wrong chain, sender,
transaction type, amount, compute limit or fee bound. It returns a signed
TransactionV2 but never submits it.

The API prepares a native HBT transfer using `hb_feePolicy` and
`hb_getTransactionCount`; after signing it persists the accepted decision and
exact transaction ID with fsync before the only state-changing RPC,
`hb_sendRawTransactionV2`. An uncertain submission is retained and is never
automatically replaced by a new transaction.

## Fail-closed requirements

- only chain 4735490 or 4735489;
- dedicated faucet address, never founder/validator/internal service identity;
- protected destinations configured explicitly;
- append-only audit directory not group/world writable;
- recipient cooldown, daily cap and global reserve;
- per-source API limiter plus durable economic limits;
- signer only on explicit loopback;
- RPC only HTTPS or explicit loopback HTTP;
- signer amount/fee/compute bounds are independent from API policy;
- no API field accepts or returns a private key.

Mainnet deployment is a later operational gate. Before it is started, the exact
dedicated faucet reserve must be frozen and funded from the approved founder
allocation; the imported legacy 450 HBT balance is not a faucet source.

This F2 implementation does not create a key, fund an account, start a service,
sign a production transaction or broadcast a transaction by itself.
