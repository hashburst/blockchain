# HashBurst Wallet: offline custody/signing candidate

Status: source candidate; not a deployed wallet release. The executable is built
from cmd/hashburst-wallet and can be distributed alongside a full node. It has no
HTTP listener. Explicit online commands use public RPC; custody and signing stay local. The founder private key must never be installed on a
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
Custody commands: create, encrypt, verify, sign-transfer.
Online commands: prepare-transfer, submit-transfer, receipt.
The signing draft must be an unsigned protocolv2 HBT_TRANSFER (type 1, version 2),
with explicit chain_id, sender, to, value_units, sequence, compute_limit and
max_fee_units; no data, signature or id. Only chain IDs 4735490/4735489 are allowed.
The wrapper shows recipient/value/fee/sequence and requires SIGN <chain ID>.
Sequence and fees must come from a trusted node before constructing the draft;
prepare-transfer fetches them and applies the user fee cap, without independently certifying the RPC server. A valid signature is not proof
of an executable or finalized transfer. Signed drafts may be broadcast by anyone
who possesses them; store them privately too.

Native protocol units are 1e8 per HBT. They must not be confused with the 1e18 wei
representation at the Ethereum API. This tool signs native protocol envelopes,
not Ethereum transactions. Mainnet allocation and APoW activation are separate.

Use the HVM native wallet candidate workflow for this exact source commit to
check Go 1.25.7 tests and platform builds. Earlier run success does not certify
a later binary. Tests include RPC mismatch, receipt validation and ambiguous-send handling.
Do not migrate production custody or sign transfers using an unvalidated build.

## Direct encrypted creation

The `create` command generates a fresh secp256k1 key and writes only the verified
password-encrypted keystore. It prints the public address and chain ID, creates
no allocation and sends no transaction. A seed phrase is not generated. Restore
requires both this keystore and its password; test restoration before funding.

On Linux or macOS, create a private directory, then use the interactive wrapper:

```
mkdir -m 700 wallet-testnet
python3 prompt.py create --binary ./hashburst-wallet --chain-id 4735490 --out wallet-testnet/wallet.json
```

Use a different output directory and chain ID 4735489 for mainnet. Keystore V3 is
network-neutral; chain enforcement occurs during signing, not inside the key file.
No browser download or server custody is involved. Native transfer signing only;
token ABI interaction and integrated node submission remain separate work.
Windows custody refuses operations until a Windows ACL backend is implemented.
Do not bypass that guard by assuming chmod has POSIX semantics on Windows.

## Native full-node transaction workflow

Three explicit online commands use the native `/rpc` interface (not the EVM
endpoint). They never unlock a keystore. HTTPS or an explicit loopback IP is
required; redirects are refused and the chain ID is checked before use.

1. `prepare-transfer --rpc URL --chain-id 4735490 --from ADDRESS --to ADDRESS
   --value-units INTEGER --compute-limit INTEGER --fee-cap-units INTEGER
   --out private/draft.json` obtains the pending native sequence and fee policy,
   applies the user's fee cap, and saves an unsigned draft exclusively.
2. Review and sign that draft using `prompt.py sign-transfer` on the custody
   device. No RPC is used by signing. Keep the resulting signed JSON and hash.
3. `submit-transfer --rpc URL --chain-id 4735490 --in private/signed.json
   --out private/attempt.json --confirm-tx 0xHASH` verifies the signature, checks
   for an existing receipt and records a durable attempt before posting once.
4. `receipt --rpc URL --chain-id 4735490 --in private/signed.json` checks the
   returned transaction ID, success and signed fee/compute limits. It sends no
   transaction. A missing receipt is pending/unknown, not proof of rejection.

After a submission error retain both files and use `receipt`. Do not delete the
attempt marker to work around a timeout: admission may already have succeeded.
Receipt validation is against a node's report; it is not an independent QC proof.
Balances and arbitrary token ABI calls are not implemented by these commands.
The configured native ingress may reject writes; do not bypass its allowlist.
