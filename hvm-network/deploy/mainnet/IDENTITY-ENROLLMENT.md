# Mainnet public validator enrollment

The `hvm-mainnet-identity` command prepares signed public enrollment data only.
It does not assemble a validator checkpoint, fund bonds, submit transactions,
start services or approve the supplied protocol policy.

Run generation on each selected validator host with a reviewed, complete
ProtocolV2Config JSON for chain 4735489. Do not substitute the testnet protocol.
The approved economic import and finite activation heights are required.
Use the host's actual TEP X25519 public key; never supply the private key.

```
go build -o hvm-mainnet-identity ./cmd/hvm-mainnet-identity
./hvm-mainnet-identity generate --protocol protocol.json \
  --out /etc/hashburst-hvm-mainnet/identity \
  --node-id "$MAINNET_NODE_ID" --ip "$ADVERTISED_IPV4" \
  --tep-public-key "$TEP_PUBLIC_KEY_HEX"
./hvm-mainnet-identity verify --protocol protocol.json \
  --public /etc/hashburst-hvm-mainnet/identity/public.json
```

The parent directory must already exist and have a canonical path. The command
exclusively creates a mode-0700 directory and mode-0600 private files; it never
overwrites an identity. If an I/O failure leaves a partial directory, preserve it
for inspection instead of automatically regenerating keys. Keys are unencrypted
local runtime material protected by filesystem permissions; do not copy them to
GitHub or the coordinator. Export only public.json.

The signed enrollment contains node and validator registration transactions,
a hash of the typed protocol configuration, and separate consensus and P2P
proofs of possession over a domain-separated digest. Changing the chain,
protocol or registration invalidates verification. Registration uses operator
sequence zero, the protocol minimum bond and an explicit 80000 compute budget.
The checkpoint assembler must verify sequence, actual execution cost, available
funds and global uniqueness before consuming it. This command does not provide
that assembler or authorize funding.

TEP public identity is operator-attested. It is not a proof of X25519 private-key
possession or admission to the TEP federation. Confirm the identity against the
federation before accepting a production validator set.

Production protocol values and host roles remain separate activation inputs.
Tests use synthetic local identities and do not constitute production enrollment.
