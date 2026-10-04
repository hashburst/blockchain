# Approved legacy reference, chain 1337

On 2026-10-04 the owner selected the branch served by 64.31.4.9 and corroborated by the block hashes reported by 77.90.188.153 and 77.90.188.154. This is an explicit migration reference decision, not a claim that a count of replicas establishes consensus finality.

The reference is height 9 (10 blocks), hash `0000ca5a560580d0a882a1e46808331e414d073dbc2b0125f5310a6a6a81f586`. The captured files are `/var/lib/hashburst/blockchain.dat` and `blockchain.idx`. Nine rewards total 45,000,000,000 atomic units (450 HBT) for `0x534746AC40019Ec4E19eb3D12d5F716D4f8b7e3e`. Nine other transactions are zero-value node registrations. This manifest contains no founder allocation, faucet budget or future issuance.

The .157 branch shares blocks 0–6 but assigns the rewards of blocks 7–9 to a different recipient. Both sets of rewards must never be summed. Preserve the entire .157 ledger before repair. API `miner` metadata varies by serving node even for matching blocks; use the actual reward transaction receiver.

Offline evidence checks have verified file digests, index/frame bounds, parent linkage, monotonic timestamps, legacy SHA-256 block hashes, Keccak transaction IDs and recoverable secp256k1 signatures. Each branch has 18 transaction IDs and 9 non-system signatures. PoH checks used the legacy 400,000 SHA-512 iterations per block. These checks refer only to the captured data, not deployed binary provenance or an executed spending freeze.

## Next operation

Run `python3 inspect-repair.py` on the administration Mac. It asks for SSH authentication separately on .155 and .157 and returns a new local report. It does not restart any service, install a binary, execute the legacy binary, alter files remotely, or acquire private keys. It reads public ledger data only under a 16 MiB limit, selected public runtime settings, systemd status and the installed binary digest. A failed legacy API on .155 does not establish that its disk state is missing or corrupt.

## Repair and activation boundaries

No remote repair is implemented or executed by this change. The approved reference must not be installed by overwriting live dat/idx files. First determine the actual legacy service configuration and binary, retain old ledgers and configuration, stop only legacy writers in a controlled maintenance window, and publish a validated generation. Preserve P2P/wallet/TEP identity, HB-Files/IPFS state and all HVM services. Do not silently create genesis when an existing ledger is missing or corrupt.

A normal legacy runtime may produce or adopt blocks after the selected height. Restoring the reference pair is not a spending freeze. Before mainnet activation, implement and test the canonical managed legacy network's freeze across local submission, peer ingress, block production, chain adoption and restarts. An HTTP-only restriction is insufficient. Independent third-party forks cannot be globally stopped by this procedure.

Mainnet 4735489 must use separate genesis, state and signing identities/journals from legacy 1337 and testnet 4735490. The manifest deliberately denies mainnet activation until repair, freeze proof, full economic manifest and mainnet identity/genesis commitments exist. Neither choosing a reference nor merging this document activates a network.
