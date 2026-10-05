# HVM Mainnet 4735489 — runtime foundation, not an activation release

The five managed legacy 1337 services now run a terminal read-only archive. Testnet 4735490 remains separate. The separate executable
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

1. Implement the approved legacy terminal import in the mainnet bootstrap;
   migration is decided, but no mainnet allocation has been executed.
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
Founder allocation and legacy import amounts are recorded below. A complete
protocol/validator manifest and authenticated bootstrap are still required.

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

## Managed legacy freeze accepted 2026-10-05

`legacy-migration.candidate.json` binds the exact SHA256 of the five-host freeze
report to terminal height 10, file commitments, recipient and 45,000,000,000
native units (450 HBT). The source nullifier depends on the source boundary,
not the report timestamp: a refreshed observation cannot authorize another import.
The .153 HA-controlled master transition is recorded in the supplied report.

`migration_model.py` is an OFFLINE accounting model, not a production importer.
Its tests verify reconciliation, altered-recipient/amount rejection, overflow,
wrong-chain rejection and duplicate rejection after JSON state reload. This is
not a crash/restart test of the mainnet daemon and does not credit any live wallet.
Run: `python3 -m unittest discover -s deploy/mainnet -p test_migration_model.py -v`.

Production integration must commit consumed sources and balances atomically to
the same authenticated consensus state/checkpoint and enforce the import rule
on replay. A sidecar receipt or local flag alone is insufficient. Do not change
`activation_allowed` to true to bypass this integration.

The approved founder billion is additional to 450 imported HBT: the proposed
initial total is 1,000,000,450 HBT before rewards. Zero additional migration
issuance refers to the 450-HBT transfer, not to the separate founder allocation.
Legacy copies outside the managed fleet are outside the freeze evidence scope.

Public sites must currently describe legacy as archived, testnet separately,
and mainnet as not activated. No mainnet balances, availability or release
readiness may be claimed from the offline accounting tests.
