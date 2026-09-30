# APoW, PoH and BFT integration v1

Status: implementation candidate, disabled unless `protocol.apow` is explicitly
configured. Not an activation or migration of legacy 1337, testnet 4735490 or
mainnet 4735489. This proposal changes consensus at an agreed activation height.

## Four separate responsibilities

1. A miner produces a SHA-256 proof below the specified target and signs it with
   a separate secp256k1 work key. The challenge commits chain ID, height, parent
   hash, PoH output, target bits, epoch start, author, beneficiary and nonce.
2. The existing deterministic PoH transition orders successive blocks. This
   change neither upgrades its existing 64-bit representation nor claims it is
   a secure wall-clock oracle or a new verifiable delay function.
3. The existing scheduled BFT proposer chooses a valid work candidate, constructs
   and executes the block, then obtains the existing prevote/precommit quorum.
   Hash power does not grant validator voting power. BFT quorum and persistent
   double-sign/lock protections are unchanged.
4. Exactly one 50 native-HBT subsidy goes to the beneficiary signed into the work
   proof, only when the containing block is finalized. Miner, beneficiary and
   proposer may differ. There is no reward per submitted proof, node or vote.
   Transaction fees continue through the existing execution/accounting rules;
   they are separate from the fixed subsidy.

A relayer cannot substitute its own beneficiary. A proof is specific to a chain,
height and parent. A certified proposal keeps its proof and beneficiary across
BFT rounds. Competing proofs do not cause multiple rewards at the same height.
There is no PoW fallback that bypasses BFT and no BFT fallback that omits work
once activation is reached. A lack of work pauses block production while the
pacemaker continues; successful operation requires miners to supply new jobs.

## Deterministic adaptive difficulty

`initial_bits`, `min_bits`, `max_bits`, `window`, `target_seconds` are pinned
consensus parameters. The target is SHA256 < 2^(256-bits). Every window,
validators compare the finalized parent timestamp to the epoch start inherited
from verified parents. Below half the target duration, bits increase by one;
above twice the duration, bits decrease by one; otherwise unchanged. Changes
are bounded by min/max. No floating point, local telemetry, AI prediction or
unverified miner timestamp enters target selection.

This is a new explicit APoW profile, not a claim of equivalence to every older
APoW document. BFT delay is included in measured duration. Timestamp manipulation,
parameter calibration, miner availability and censorship/fair allocation require
adversarial testnet evaluation before mainnet activation. The proposer can choose
any valid proof: lowest digest in the local bounded pool is a policy, not a global
fairness guarantee. A miner may also be a validator; majority hash power alone
cannot override a BFT quorum, and proof of work alone does not prevent an
independently spendable legacy balance from being copied into another chain.

## Runtime integration

Version 4 commits the work digest and signature in addition to all version-3 EVM
commitments. Versions 1–3 and nil-APoW config JSON remain unchanged. Structural
validation validates the challenge, target, signature and reward on ordinary
append, full-chain verification and replay. Ethereum block/events continue to
work for version 4. Work signatures are canonical low-S with recovery IDs 0/1.

GET `/apow` returns the next job. POST `/apow` accepts a proof (2048-byte bound),
and relays it through the existing bounded consensus transport. Both operations
require a loopback client; public EVM ingress does not expose them. At most one
candidate is retained per parent, and stale work is rejected. `hvm-apow-miner`
mines a single bounded job, using an owner-only private key file separate from
validator keys. Use SSH tunnels for remote miners. It is a reference worker,
not a pool payout service or an optimized industrial miner.

## Reproducible checks and deployment gates

From repository root with Go 1.25.7:

```
go -C hvm-network test ./...
go -C hvm-network test -race ./blockchain -run TestAPoW -count=1
go -C hvm-network build ./cmd/hvm-apow-miner
cd evm/strict && npm ci --ignore-scripts && npm run build
cd ../execution && go test ./...
```

Tests cover distinct work/beneficiary identities, four-instance QC finalization,
subsidy, stale/altered proofs, bounded retarget, cancellation, no-work startup,
loopback-only admission, certified reproposals and persistent reopen with journal
preservation. This is not a four-VPS acceptance report.

Before distributing an enabled configuration: review this consensus rule;
choose measured difficulty/timing parameters; define and test a configuration-pin
migration preserving recovery locks and signing journals; test four real validators
plus observer under loss and restart; check fixed-height agreement, rewards and
EVM execution; then publish a separate mainnet manifest. Existing configuration
pins must not be rewritten manually. The dedicated testnet-only `--migrate-apow`
command verifies both configurations and replays the candidate under an exclusive
state lock; it records an immutable intent and can resume the same pin/config
transition. It preserves chain data, signing journals and the recovery snapshot.
The existing EVM-only migration must not be reused for APoW.

An optional `apow.gas_limit` activates with APoW; historical gas semantics retain
`evm.gas_limit`. Replacing that historic value is rejected by migration. The
2,000,000-gas strict fixture is a tested experimental capacity, not an approved
mainnet setting. See `deploy/testnet/apow/README_IT.md` for the live preflight.

Founder allocation, faucet policy, supply schedule and legacy economic migration
are separate consensus/economic decisions, not implicitly executed by this patch.
