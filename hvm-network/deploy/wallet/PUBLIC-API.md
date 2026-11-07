# HVM Native Wallet Public API Contract

Status: activation candidate.

HashBurst exposes two complementary wallet surfaces:

1. MetaMask/Web3 through the HVM EVM-compatible JSON-RPC interface.
2. The native HBT wallet, which signs protocol-v2 native envelopes locally.

They share the same secp256k1/EVM-style address space but native HBT protocol units (1e8/HBT) MUST NOT be confused with EVM wei (1e18/HBT representation).

## Custody rule

Public HashBurst infrastructure is non-custodial. Production user private keys are generated, encrypted and used on the user's device. The public API MUST NOT accept a private key, seed phrase, mnemonic, wallet password or encrypted keystore upload.

The existing `hashburst-wallet create` command remains the authoritative native key generator. Web applications MUST use audited client-side cryptography and provide an export/backup flow before funds are received.

## Public API v1

Network-qualified responses MUST include `chain_id`.

- `GET /hvm/v1/network` — chain metadata, native units, RPC/explorer/faucet capabilities.
- `POST /hvm/v1/address/validate` — validate/checksum an address; no key material.
- `GET /hvm/v1/account/{address}` — finalized balance, pending sequence/nonce and finalized height.
- `GET /hvm/v1/fees` — current native fee/compute policy and EVM fee metadata.
- `POST /hvm/v1/transactions/prepare` — construct an unsigned native HBT transfer using authoritative sequence and fee policy.
- `POST /hvm/v1/transactions/broadcast` — accept only a fully signed protocol-v2 envelope and return transaction ID/admission status.
- `GET /hvm/v1/transactions/{id}` — pending/finalized/rejected/unknown status and receipt when available.
- `GET /hvm/v1/faucet/policy` — public limits/cooldown/funding status.
- `POST /hvm/v1/faucet/claim` — request a grant to a caller-supplied address; returns decision and transaction ID when accepted.

## Security requirements

- strict body limits and JSON schema;
- chain-ID binding and replay rejection;
- address and amount validation;
- no CORS wildcard on state-changing routes;
- per-IP and per-address rate limits for faucet;
- idempotency key on transaction preparation/broadcast and faucet claim;
- stable error codes with no secret/internal path leakage;
- public gateway allowlist: no validator/admin/APoW control endpoints;
- finalized state is distinguished from pending state;
- append-only audit for faucet and broadcast admission;
- TLS at the public reverse proxy.

## Node Installer

The node-installer may enable the public read/broadcast gateway only for roles explicitly configured for RPC/observer service. Validator private control surfaces remain loopback/private. Faucet signer is a separate optional service with its own secret boundary and funded account.

Marker: HVM_NATIVE_WALLET_PUBLIC_API_V1_DEFINED
