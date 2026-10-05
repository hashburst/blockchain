# HVM Network testnet acceptance - 29 September 2026

Scope: testnet 4735490, legacy 1337 unchanged, mainnet 4735489 not activated.

The operator ran the browser MetaMask canary and the independent public RPC
verifier. The supplied artifacts are preserved here without reconstructed data.
The wallet proof records a transfer, contract deployment and call, successful
receipts, a newHeads notification, a matching contract log and unsubscribe.
The public verifier checks exported receipts against RPC, canonical finalized
blocks, sender and chain ID, deployed bytecode, storage slot 0 = 42, canonical
logs and matching WebSocket evidence.

- Public verification head: 104804.
- Contract: 0xaecadbfa3565117d218ec1c4913d3c807728ad0f.
- Call block: 103643; receipt and subscription block hash agree.
- Historical API verification: fixed height 103597, six read methods, no send.
- Browser completion: 2026-09-29T17:29:27.317Z.

The historical report's `metamask_certified: false` scopes that read-only test;
it does not negate the separate successful wallet acceptance artifact.

This is acceptance of the specified canary, not exhaustive Ethereum conformance,
all wallets/features, an archive node, decentralization or mainnet readiness.
Recent state retention is 256 canonical EVM blocks. Log events here use LOG0;
the test does not establish all indexed-event/filter combinations.

The observed loss of finality at 97163 coincided with v3 and v4 being inactive
after a boot boundary. The operator started the existing services and enabled
automatic startup. No validator journal reset or quorum relaxation was used.
Observer verification then passed, preserving identity/configuration/pin/journals.

Reproduce against the public testnet, without sending another transaction:

    python3 ../../deploy/testnet/evm/recovery/verify-public.py --wallet-proof wallet-proof.json

Historical snapshots beyond the retention window are unavailable by design;
rerun verify-history.py for a new recent height rather than claiming archive support.

Independent review in this session loaded the three original JSON files, checked
receipt/subscription consistency, and reran the public verifier successfully:
`PUBLIC_EVM_ACCEPTANCE_OK` and `METAMASK_RECEIPTS_LOGS_AND_SUBSCRIPTION_PROOF_OK`.
The fresh read-only result is saved as GATE-independent-recheck.json.
