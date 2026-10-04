# Consolidated testnet acceptance

PR 28 merged into PR 27 as 3e5e1b24bccaf362b962ac9b173a480eda47da07;
PR 27 merged into master as 9335dbfdf2ce2390c682a949ea4aea9713822bb6.
Tested tree: abbadd532a360e82fadf875209ff726ab9f86a17.
Deployed runtime source: 2719806647f25012a8d6b868a645c0341b0442c1.
Deployed runtime SHA256: 30a0cd390af000019577f06bcf97ce54959808aa4447ab318ef669cf8aa9be0e.

Five testnet nodes agreed at height 161493, then advanced. Observer incremental
recovery replayed 320 blocks in 6563 ms; validator v4 replayed 523 blocks from
checkpoint 160768 in 23024 ms. These are recovery metrics, not total upgrade
durations. Its new precommit appears in the finalized certificate at 161489.
64 historical commitments remained equal, allowing only certificate vote order
normalization. Configuration, identity pins and journal prefixes passed the
remote verifier. 64 sampled APoW blocks carry unique 50 HBT reward transactions,
3200 HBT in total. This does not establish full balance or supply conservation.

## Independent certificate audit

`collect-qc-evidence.py` obtains public sets for the previously accepted roots.
The RPC uses the current registry projected to the requested height; it is NOT
a historical-state oracle. A root mismatch stops collection and requires a
historical replay export. Never replace the accepted root to make this pass.
`hvm-qc-audit` checks key/address/validator-ID binding, set root, individual
signatures, duplicate votes, quorum power and external block/chain/height.
It runs offline, independently of the live node, reusing protocol primitives.
Its success is relative to the supplied anchors, not an audit from genesis.

Run from the module: `go run ./cmd/hvm-qc-audit < qc-evidence.json`.
Public reports are not private key material. Never upload signing keys.

## Release and mainnet boundary

The package workflow produces source-bound candidate artifacts for Linux and
macOS amd64/arm64 and Windows amd64 client tools. Windows full-node runtime
still depends on Unix-specific state locking and is not certified here.
No production service is changed by this workflow. Existing pinned installer
manifests remain immutable. Do not run the obsolete deploy/publish coordinator.

Mainnet 4735489 must use separate genesis, identities, state, signing journals,
configuration and endpoints from testnet 4735490 and legacy 1337. Legacy source
ledger, agreed snapshot boundary, ownership mapping and consensus-enforced
spending freeze evidence have not been supplied. They cannot be inferred from
testnet reports. The economic manifest remains non-activatable until these
inputs, balance reconciliation, validator identities and protocol parameters
are verified. Disabling an API is not a spending freeze. No stable mainnet
release or energy reduction is claimed by these testnet results.
