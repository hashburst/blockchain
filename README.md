# HashBurst Blockchain

Public Distributed HashBurst Blockchain.

HashBurst is a distributed blockchain framework based on Proof of History (PoH), with implementations and integration components written in C++, Python, PHP, Go and Bash.

The project includes blockchain data management, node integration, wallet and ledger handling, cryptographic verification, mining infrastructure integration and network automation.

## Official Node Installer

Production deployment of HashBurst blockchain and sovereign storage nodes is handled by the official HashBurst Node Installer:

[HashBurst Node Installer](https://github.com/hashburst/node-installer)

Installer releases are versioned independently from HVM Network; consult the release list below.

Release packages and release notes:

[HashBurst Node Installer Releases](https://github.com/hashburst/node-installer/releases)

The installer provides deployment support for HashBurst blockchain nodes, HB-Files sovereign storage, private IPFS integration, storage aggregation and the current HashBurst node service architecture.

## Architecture

The HashBurst framework combines several components:

* Proof of History based blockchain processing
* Distributed ledger management
* User and wallet data management
* SHA-512 hashing
* CRC32b integrity verification
* AES-256-CBC based encryption components
* Blockchain API integration
* Mining node integration
* Cluster and sub-account management
* Network performance monitoring
* Reinforcement Learning based mining optimization
* Automated miner deployment and release management

## HVM Network: status and chain IDs

Status recorded on 2026-10-05; this is an acceptance summary, not a live monitor.

| Network | Chain ID | Verified status |
| --- | --- | --- |
| Legacy | 1337 | Five managed nodes serve the immutable terminal archive; 11 blocks, final index 10. |
| HVM testnet | 4735490 | Separate running network; five-node agreement, incremental recovery and sampled certificate checks completed. |
| HVM mainnet | 4735489 | Not activated. Bootstrap, economic import and runtime acceptance remain required. |

The terminal legacy block moves exactly **450 HBT** to
`0xd1Da8D04D767685e53440DbC56803aF350A65333` with **zero new issuance**.
Managed-fleet archive acceptance does not establish a freeze of unmanaged forks.
The Go genesis import now commits balances and consumed-source identity together
and preserves both through replay and authenticated checkpoints. It remains
non-activatable until the production validator checkpoint and runtime acceptance
are complete. See [genesis import](hvm-network/deploy/mainnet/GENESIS-IMPORT.md).

## HVM ledger and recovery

`hvm-network` stores block payloads in `blockchain.dat` and positional records
in `blockchain.idx`. Each index record is 20 bytes:
`[BlockIndex uint64][FileOffset uint64][BlockSize uint32]` in big-endian order.
Historical access is on demand with bounded caching. Existing supported payload
formats are detected by the runtime; renaming files is not a format conversion.
`masterData.hbx` belongs to older components and is not the HVM runtime ledger.

Authenticated local checkpoints and incremental replay reduce repeated recovery
work. Periodic checkpoint persistence is separate from durable finalized blocks
and signing journals. Memory use is bounded by more than the cache alone; no
zero-RAM, zero-allocation or measured energy-saving claim is made.
PoH verification, BFT certificates and APoW rules are defined by the Go protocol;
a simple timestamp hash is not a replacement for consensus verification.

## Releases and websites

Published candidate: [HVM v0.7.0-rc.2](https://github.com/hashburst/blockchain/releases/tag/hvm-network-v0.7.0-rc.2).
Next candidate: **hvm-network-v0.7.0-rc.3**, draft only, including the terminal
archive and migration preparation. A final release requires mainnet acceptance.
Linux/macOS amd64 and arm64 packages are cross-built; Windows amd64 includes
wallet and certificate auditor only. Compilation is not a platform runtime certification.

Website sources are prepared under [deploy/web](hvm-network/deploy/web/README.md):
**HVM Network** in English for https://blockchainapi.one/hashburst and
**hashburst.io** in Italian and English. Repository content is not evidence of
website deployment. Publication remains gated and existing VPS services stay unchanged.

See [release notes](hvm-network/deploy/release/RC3-PREPARATION.md),
[mainnet preparation](hvm-network/deploy/mainnet/README.md) and
[migration candidate](hvm-network/deploy/mainnet/legacy-migration.candidate.json).

## Earlier implementations

The PHP, Python and C++ examples document earlier framework designs. Their
filesystem layouts, timestamp-hash examples and encryption examples do not
specify HVM consensus. The current Go implementation is under `hvm-network`;
its protocol, replay and state-root checks govern HVM blocks.

## Go Components

Go is used for HashBurst node, mining and infrastructure automation.

Components include:

* API authentication
* HashBurst API communication
* miner deployment
* miner lifecycle management
* network testing
* node registration
* cluster configuration
* release automation

Relevant functions in HashBurst components include operations such as:

```text
verifyWithHashburst(...)
downloadMiner(...)
startMiner(...)
testNetworkSpeed(...)
```

## API Authentication

HashBurst infrastructure includes API-based authentication and node verification.

Authentication workflows can use:

```text
email
API key
referral code
node identity
```

API credentials are used to associate infrastructure components with registered HashBurst users and nodes.

Sensitive credentials must not be committed to the repository.

## Network Performance Testing

HashBurst node and mining components include network testing functionality.

Network checks can be used to measure:

* connectivity
* latency
* endpoint availability
* network suitability for mining or node operations

These checks allow node software to validate connectivity before starting dependent services.

## Mining Infrastructure

HashBurst includes integration components for mining infrastructure.

Configuration parameters can include:

```text
MINER
ALGO
ENDPOINT_POOL
PORT
ACCOUNT.SUBACCOUNT
COIN
```

These parameters can be used to generate miner configurations and associate workers with specific pools, accounts and mining strategies.

Mining infrastructure is separate from the HashBurst sovereign storage network.

## Mining Cluster and Sub-Account Management

HashBurst supports generation of configuration data for miner clusters composed of workers and sub-accounts.

Cluster configuration can associate individual nodes with:

* API credentials
* pools
* mining algorithms
* wallets
* sub-accounts
* worker identities
* network endpoints

This provides a consistent configuration mechanism for distributed mining infrastructure.

## Reinforcement Learning for Mining Optimization

HashBurst research and development includes the use of Reinforcement Learning models implemented with PyTorch for mining optimization.

Optimization targets can include:

* pool selection
* node resource allocation
* mining strategy selection
* throughput analysis
* performance feedback
* dynamic configuration

The optimization layer operates on performance information produced by the HashBurst infrastructure.

## Miner Release Automation

Go components can automate miner deployment using releases published through GitHub.

The automation workflow can:

1. determine the required miner release;
2. download the miner package;
3. prepare its configuration;
4. associate the node with HashBurst credentials;
5. start the mining process;
6. monitor the process and network connectivity.

This functionality supports repeatable miner deployment across HashBurst nodes.

## Node Deployment

For production HashBurst node deployment, use the official installer rather than manually reconstructing services from individual repository examples:

https://github.com/hashburst/node-installer

The current stable deployment line is:

```text
v2.1.2
```

The installer includes the current service definitions, node binaries, HB-Files components, IPFS integration, storage aggregation and release validation tests.

## Network Ports

Current HashBurst node infrastructure separates mining and storage aggregation services.

```text
8091   Storage node public summary
8093   Mining aggregator
8094   Storage network aggregator
18094  Temporary storage aggregator validation port
```

Port `8093` is reserved for mining aggregation and must not be reused by the storage aggregator.

## Sovereign Storage

The current HashBurst node architecture includes HB-Files sovereign storage backed by private IPFS infrastructure.

Storage nodes can operate with different roles and capacity classifications.

Typical roles include:

```text
primary
secondary
edge
```

Capacity classes include:

```text
committable
best-effort
unknown
```

Edge capacity must not be treated as guaranteed sellable capacity.

Offline nodes without a valid configured role or capacity class are classified as `unknown` rather than implicitly becoming `committable`.

For the authoritative implementation and deployment configuration, refer to:

[HashBurst Node Installer](https://github.com/hashburst/node-installer)

## Repository Scope

This repository contains HashBurst blockchain framework components and implementation references.

Production deployment configuration is maintained separately in the official Node Installer repository so that blockchain source development and infrastructure deployment remain independently versioned.

## Related Repository

Official HashBurst Node Installer:

https://github.com/hashburst/node-installer

Stable releases:

https://github.com/hashburst/node-installer/releases

## Mainnet activation inputs

[Production activation inputs](hvm-network/deploy/mainnet/ACTIVATION-INPUTS.md)
records the public identity and protocol data still required for the validator
checkpoint. The economic genesis implementation is merged; production mainnet
activation and final website publication have not been performed.
