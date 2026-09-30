# HashBurst Wallet: offline custody/signing candidate

Status: source candidate; not a deployed wallet release. The executable is built
from cmd/hashburst-wallet and can be distributed alongside a full node. It has no
HTTP listener or RPC client. The founder private key must never be installed on a
validator/miner/observer or submitted to a dashboard. No seed phrase is created.

Custody uses the existing Web3 v3 encrypted keystore (scrypt N=262144,r=8,p=1).
A password alone cannot recover a lost keystore; a keystore alone cannot recover
a lost password. Back up both separately, with offline protected copies and a
successful independent unlock test. Keep the original wallet.key until backup
and restore are verified; this tool deliberately never removes it. Its continued
presence means the private key is still accessible in plaintext on this Mac.
No claim of secure erasure on APFS/SSD is made.

The password is entered through Python getpass and sent via an anonymous pipe,
never a command-line argument or environment variable. Keys/files are owner-only;
outputs use exclusive creation and fsync. Address verification and read-back
unlock are required before encryption is declared successful. An encrypted V3
key itself is network-neutral; every signer invocation requires the expected
public address and chain ID. Keep separate keys for testnet and mainnet.

Build with Go 1.25.7: go build -o hashburst-wallet ./cmd/hashburst-wallet
Run deploy/wallet/prompt.py --help for the wrapper interface.
Commands: encrypt, verify, sign-transfer. No broadcast command is included.
The signing draft must be an unsigned protocolv2 HBT_TRANSFER (type 1, version 2),
with explicit chain_id, sender, to, value_units, sequence, compute_limit and
max_fee_units; no data, signature or id. Only chain IDs 4735490/4735489 are allowed.
The wrapper shows recipient/value/fee/sequence and requires SIGN <chain ID>.
Sequence and fees must come from a trusted node before constructing the draft;
this version neither fetches nor certifies them. A valid signature is not proof
of an executable or finalized transfer. Signed drafts may be broadcast by anyone
who possesses them; store them privately too.

Native protocol units are 1e8 per HBT. They must not be confused with the 1e18 wei
representation at the Ethereum API. This tool signs native protocol envelopes,
not Ethereum transactions. Mainnet allocation and APoW activation are separate.

Validation is pending execution of the Go tests in a Go 1.25.7 environment.
The HVM native wallet candidate GitHub workflow runs those tests on branch push;
a workflow definition is not evidence that its run succeeded.
Do not migrate production custody or sign transfers using an unvalidated build.
