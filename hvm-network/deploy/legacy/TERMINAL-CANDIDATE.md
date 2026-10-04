# Legacy terminal candidate: not a mainnet activation

The owner-approved chain-1337 prefix is ten blocks, indexes 0..9. The new
terminal artifact has eleven blocks, indexes 0..10. The dedicated terminal
verifier preserves legacy hashing, PoH and PoW and requires exactly one owner
signed transfer, 450 HBT, to the pinned founder destination. There is no reward.
The transfer data binds the approved source hash and target chain 4735489.
Ordinary legacy and HVM consensus validation is NOT relaxed.

`hvm-legacy-close --source PATH --check-source` verifies the pinned dat/idx
checksums, index framing, genesis, transaction signatures, PoH, PoW and the
single-owner 450 HBT projection. It does not read a private key or write state.

Building a candidate requires --authorize-terminal-transfer-450, --source,
--out and --keystore, with the V3 password on an anonymous stdin pipe.
The output must be a NEW generation outside live storage. Failed staging
folders are retained. Existing output is refused; inspect/reuse it rather than
signing another terminal block. The output pair preserves the original ten
frames and appends the terminal frame. Data, index, manifest and directories
are synced before/after publication of the separate generation.

The generated manifest records file hashes, terminal hash and state root.
Its activation_allowed, fleet_freeze_verified and mainnet_import_executed
remain false. It is not a final mainnet genesis or an import execution receipt.

`hvm-legacy-archive --directory PATH --dat-sha256 HASH --idx-sha256 HASH
--terminal-hash HASH --check` validates all eleven blocks and supply without
listening. The three commitments must come from the independently accepted
manifest, not from untrusted peers. Without --check it serves loopback HTTP
read endpoints /api/health, /api/blocks, /api/transactions, /api/balances.
All mutating HTTP methods are rejected. It starts no miner, P2P synchronization,
registration worker, transaction pool or writer. It needs no signing key.
Other legacy dashboard/storage routes are not implemented here and require
explicit routing review before replacing the existing daemon.

## Deployment gates

1. CI and candidate review; preserve original ledgers, configuration and identity.
2. Sign once on the source key-holding machine; verify the produced generation
   independently and pin its three commitments.
3. Stage the archive on an unused loopback port. Compare all blocks and balances,
   probe rejected mutations and verify unchanged data/index hashes after restart.
4. Switch managed legacy services one host at a time, keeping HVM testnet and
   HB-Files/IPFS untouched. Read-only filesystem/service permissions are required.
5. Record all managed legacy hosts running the accepted archive, with no old
   writer/miner/sync service. This does not prevent outsiders running a fork.
6. Only then reconcile the import once into a separate mainnet genesis, with
   independently reviewed founder allocation, chain identity and validator set.

No deployment, fleet-freeze attestation, mainnet import or activation is performed
by this source change. A copied balance or manifest boolean is not a freeze proof.
