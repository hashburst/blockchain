# HVM Network v0.4.0-rc.1

Release candidate. Mainnet is not active. No automatic service restart, state
migration, legacy spending freeze or founder allocation is performed.

Includes encrypted native-wallet creation/import/signing, full-node native RPC
transaction preparation/submission/receipt commands, testnet/mainnet executables,
and CPU APoW reference miner. Online wallet commands never unlock a private key.
Includes a tools-only installer for Linux and macOS and source checksums.

Verified: full Go regression suite and APoW/recovery race tests; wallet tests on
Linux/macOS. Cross-platform compilation does not replace deployment acceptance.

Not yet completed: live APoW rollout and rewards/restart evidence, enforced legacy
freeze and audited snapshot import, production mainnet checkpoint and activation,
Windows locking/ACL port, desktop wallet and token UI, GPU/ASIC/Stratum backends.
The economic draft records approved policy but is not an activation input.

## Installation

Download the matching archive, extract it, enter its directory, verify SHA256SUMS
(sha256sum on Linux or shasum -a 256 on macOS), then run `bash install-tools.sh`.
Tools are installed into a new source-commit-specific directory under
~/.local/hashburst-tools. No PATH changes or existing tool replacements occur.
Server/HPC/VPS service provisioning still requires the network's validated
configuration, separate identities and deployment plan. Do not replace a running
validator executable manually with these candidates.

Wallet custody remains on the user's device. Never place a founder private key
on a validator, miner, observer or dashboard. Mainnet 4735489, testnet 4735490 and
legacy 1337 are separate signing domains. No testnet funds are production assets.
