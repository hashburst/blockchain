# Founder identity recovery verification — 2026-11-07

Status: **VERIFIED — original founder identity recovered; custody blocker removed**

This record documents a local, read-only verification of the recovered native HBT wallet material. No private key, seed, password, private-key hash, or private-key-derived secret is included in this repository.

## Mainnet

- Network: mainnet
- Chain ID: 4735489
- Recovered public descriptor address: `0xd1Da8D04D767685e53440DbC56803aF350A65333`
- Address independently derived from the recovered secp256k1 private key using the HVM rule `keccak256(uncompressed_pubkey[1:])[12:]`: **MATCH**
- Address matches the founder recipient already pinned by `blockchain/mainnet_genesis.go`, the legacy terminal migration and the approved economic model: **MATCH**
- SHA-256 of recovered `public.json`: `a490464c9f58c7a2f1a19ed10796bc173bb8912280fef2a53938cdc2f4859746`

## Testnet

- Network: testnet
- Chain ID: 4735490
- Recovered public descriptor address: `0xfae06fDF4dBF53Ad922907607D517f49794841B6`
- Address independently derived from its recovered secp256k1 private key: **MATCH**
- Mainnet/testnet keys are distinct: **VERIFIED**
- SHA-256 of recovered `public.json`: `40c27be94bc990454127f352d40296e4a08ea643202c626903cff5294b65433b`

## Security boundary

The recovered `wallet.key` files remain founder-custodied signing material. They MUST NOT be committed, uploaded to validators, copied into release artifacts, placed in CI secrets, or exposed to dashboards/APIs. Mainnet funding must use the existing offline `hvm-mainnet-identity sign-funding` procedure and publish only the resulting signed transaction JSON.

## Effect on activation state

The previous material blocker — inability to prove possession of the original founder identity without replacing it — is removed.

This verification **does not itself activate mainnet**. Remaining activation gates in `ACTIVATION-INPUTS.md` still apply: production validator/observer identities, approved protocol/economic parameters, signed funding, checkpoint assembly, provisioning, fleet finality/restart acceptance, RPC acceptance, faucet policy and final release evidence.

Marker:

`HVM_FOUNDER_IDENTITY_ORIGINAL_VERIFIED chain_id=4735489 address=0xd1Da8D04D767685e53440DbC56803aF350A65333 testnet_key_separate=true`
