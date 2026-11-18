# HashBurst Mainnet — Pre-G5 Signed Runtime Upgrade Evidence

Snapshot: `2026-11-18-pre-g5-signed-runtime-upgrade`

This directory contains the sanitized forensic evidence recorded before the
controlled deployment of the G5 production-signed HVM mainnet runtime.

## Binding

- Mainnet chain ID: `4735489`
- Accepted source commit: `018468c3a77f264889d2aaef51b0e7cc4eeb8dd7`
- Accepted source tree: `4e01c033f814de660526449474250a5da3578fd3`
- Pre-upgrade runtime SHA-256: `cf111db0d141e76787b76cc319f97fa6a4608de2868d4ba0d94381ba853fb2b8`
- Target G5 signed runtime SHA-256: `960bd5490c9ae0388be012e4f806c2c90a6b39efe730a78336ffab01ba829dd0`
- Configuration digest: `86a9c1e1ed65c2e3afcaf469a3155fad94add9e88a2b90e5bc80e0c2e33b3fb3`
- Private forensic index SHA-256: `0845c119bc902558523497e852b3e0eab3f5b9f631bf33cba0715fec4aa528d2`
- Private central SHA256SUMS SHA-256: `8a83e03c0abe3188fc8b1c83fe2230ceb67555e381dca2dc4bbe0785e7a42ef3`

## Snapshot procedure

The four validator rollback snapshots were acquired using a rolling,
quiescent 1-of-4 procedure. Only one validator was intentionally stopped
at a time. The remaining three validators were checked before the target
validator was restarted, and the federation was required to return to
4/4 before continuing.

The production upgrade had **not** been executed when this evidence set
was created.

## Security boundary

The operational rollback archives remain outside Git on the Founder
Control Runner and on their source validator hosts.

This repository evidence intentionally excludes:

- validator private keys;
- P2P private keys;
- recovery/checkpoint keys;
- founder signing material;
- production release private keys;
- node configuration files;
- blockchain ledger/state files;
- consensus vote/signature state;
- system journals;
- rollback TAR archives;
- credentials and SSH material.

Only hashes, byte counts, immutable source bindings and sanitized
acceptance metadata are committed here.

The presence of a rollback archive hash in this directory does not imply
publication of the archive itself.
