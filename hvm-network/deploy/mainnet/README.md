# HVM Mainnet 4735489 — runtime foundation, not an activation release

Legacy 1337 and testnet 4735490 are unchanged. The separate executable
`cmd/hashburst-mainnet` accepts only `network=mainnet`, chain ID 4735489 and an
explicit EVM configuration. It reuses the verified state/recovery/runtime engine;
it offers no testnet migration command and never creates genesis on startup.
The testnet executable explicitly rejects a mainnet profile.

Mainnet validator paths: /etc/hashburst-hvm-mainnet and
/var/lib/hashburst-hvm-mainnet. Observer paths append -ingress. RPC is loopback
127.0.0.1:18019; P2P is 31317. Configuration pins include network, protocol,
genesis, checkpoint and identity. Symlinked data directories remain rejected.
Keys must use the corresponding mainnet configuration directory. Generate fresh
identities locally; path separation alone does not prove keys were never reused.
The same port may be used on different hosts; a validator and observer cannot
both bind these listeners on one host.

Do not copy testnet database files, journals, keys, funding allocations or runtime
pins to mainnet. The testnet bootstrap's synthetic checkpoint and reduced bond,
PoW and timing values are not an approved mainnet monetary/consensus policy.

## Required before provisioning

1. Decide whether mainnet is a new network or an audited migration of legacy
   economic state. This release assumes neither and performs neither.
2. Approve a reproducible mainnet genesis/checkpoint and protocol manifest:
   allocations and reward recipients, supply/reward policy, validator bonds and
   weights, activation heights, gas limit/base fee and consensus timing.
3. Generate fresh validator/P2P identities on the selected hosts; collect only
   public registrations. Audit the shared checkpoint and its expected hash.
4. Verify host resources and dedicated ports, provision existing approved state
   in observer mode, then coordinate validator activation.
5. Compare commitments at a common finalized height, exercise single-validator
   restart without journal rollback, publish an observer-backed gateway, and
   repeat the scoped wallet and WebSocket acceptance tests on 4735489.

An accepted testnet canary is not an instruction to create production balances.
The existing mainnet genesis/economic decisions were not supplied with the
acceptance evidence. This is the remaining input to assemble a real deployment.

## Offline usage after an approved checkpoint exists

    go build -o hashburst-mainnet ./cmd/hashburst-mainnet
    ./hashburst-mainnet --config /etc/hashburst-hvm-mainnet/node.json --provision
    ./hashburst-mainnet --config /etc/hashburst-hvm-mainnet/node.json --check

Provision is exclusive and requires the exact approved checkpoint; it is not
idempotent creation. Check/reopen preserves and verifies the existing pin and
signing journals. Do not run provision against an already pinned directory.

No mainnet service, DNS, firewall, public endpoint or genesis is activated by
this change. The dashboard explicitly marks mainnet inactive.

## Economic draft recorded 2026-09-30

`economics.draft.json` records the user's one-billion-HBT founder allocation and
50-HBT finalized APoW reward. It is not a genesis input and is not accepted as an
activation manifest. Faucet funds are a subset of founder funds, not an additional
issuance. Testnet coins are economically separate and never imported as mainnet
funds. Unlimited test funding is a replenishment policy, not an infinite integer.

The user confirmed verified legacy import with a canonical legacy spending
freeze on 2026-09-30. The founder billion is additional to imported balances.
This is a decision, not proof that a freeze occurred. Source height/hash, address
ownership mapping, enforcement on legacy nodes and supply reconciliation remain
required. An API shutdown or database copy alone cannot enforce the freeze.
Independently operated old forks cannot be erased by a mainnet migration.
