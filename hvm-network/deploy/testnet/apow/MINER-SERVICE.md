# APoW testnet miner service candidate

The miner now supports --loop, --chain-id, --generate-key and --identity-only.
Each bounded attempt obtains a fresh parent-bound job; failures wait two seconds
before fetching another job. Foreign-chain jobs are rejected before signing.
SIGTERM cancels mining; HTTP operations retain their ten-second upper bound.
A separate owner-only key is created exclusively and never overwrites an identity.
The reward beneficiary defaults to this miner's address, not the validator.

miner-service.py stage installs only the dedicated user, immutable hashed miner
release, private miner key and systemd unit. It never starts/enables the miner,
restarts a blockchain node, or modifies node configuration, state or journals.
Repeating staging reuses the same key; a conflicting unit/release is rejected.
miner-service.py start additionally requires APoW already configured on the local
healthy testnet validator. It is not a consensus migration command. Do not use
start as an activation shortcut. No automated five-node migration is included.

The unit runs one reference miner with CPUQuota=25%, Nice=15 and filesystem
hardening. Actual hashrate, target bounds and timing must be measured under this
limit; no target calibration or fairness guarantee follows from a successful
submission. A stopped/unavailable miner can stall APoW finality if no other miner
supplies work. Submission success is not a reward receipt.

Validation: miner race tests cover foreign-chain rejection, verifiable signed
work with a distinct beneficiary and non-overwriting 0600 key generation.
Python tests cover network/role/identity guards and isolated unit configuration.
Systemd installation and continuous mining have not yet been exercised on VPS.

Next activation release must coordinate all five node configurations before a
future activation height, preserve persistent recovery locks and journals,
measure mining difficulty, verify reward execution and fixed-height agreement,
and pass a single-validator restart and EVM/MetaMask regression. The passing
baseline preflight alone does not satisfy these gates.

Mainnet 4735489 remains unprovisioned. A founder public address for the approved
1,000,000,000 HBT allocation is still required. Legacy balance migration requires
a specified snapshot and a verified freeze/lock/burn policy preventing duplicate
spendable balances, or an explicit decision for an independent mainnet without
legacy import. A testnet account is not implicitly a mainnet allocation recipient.
