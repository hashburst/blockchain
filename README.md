# HashBurst Blockchain

Public Distributed HashBurst Blockchain.

HashBurst is a distributed blockchain framework based on Proof of History (PoH), with implementations and integration components written in C++, Python, PHP, Go and Bash.

The project includes blockchain data management, node integration, wallet and ledger handling, cryptographic verification, mining infrastructure integration and network automation.

## Official Node Installer

Production deployment of HashBurst blockchain and sovereign storage nodes is handled by the official HashBurst Node Installer:

[HashBurst Node Installer](https://github.com/hashburst/node-installer)

Current stable release: **v2.1.2**

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

## Proof of History

Proof of History provides a cryptographic representation of the sequence and timing of blockchain events.

A simplified HashBurst PoH operation combines transaction or block data with a timestamp and produces a cryptographic hash:

```text
proof = SHA512(data + timestamp)
```

The resulting proof can be associated with the corresponding block or transaction for chronological verification.

## Ledger Structure

The blockchain framework uses a filesystem-oriented ledger structure for persistent blockchain data.

Typical paths include:

```text
ledger/
|-- users/
|-- wallets/
`-- masterData.hbx
```

### User Data

```text
ledger/users/
```

Contains user-specific blockchain records.

User records can be encrypted using credentials associated with the account or API authorization mechanism.

### Wallet Data

```text
ledger/wallets/
```

Contains wallet information associated with blockchain users and supported networks.

### Master Ledger

```text
ledger/masterData.hbx
```

Contains the aggregated blockchain ledger data.

The framework includes mechanisms for encrypting ledger information and validating data integrity through cryptographic hashes.

## PHP Implementation

The PHP implementation provides components for:

* blockchain record creation
* Proof of History generation
* AES-256-CBC encryption and decryption
* SHA-512 hashing
* CRC32b integrity verification
* user ledger processing
* wallet management
* master ledger generation

A simplified Proof of History implementation is:

```php
function generateProofOfHistory($data)
{
    $timestamp = time();
    $combinedData = $data . $timestamp;
    $proof = hash('sha512', $combinedData);

    return [
        'data' => $data,
        'timestamp' => $timestamp,
        'proof' => $proof
    ];
}
```

Block data can then be serialized, protected through the configured cryptographic mechanism and stored in the HashBurst ledger.

## Python Implementation

The Python implementation provides equivalent blockchain processing components and cryptographic operations.

A simplified Proof of History implementation is:

```python
import hashlib
import time

def generate_proof_of_history(data):
    timestamp = str(time.time())
    combined_data = data + timestamp
    proof = hashlib.sha512(combined_data.encode()).hexdigest()

    return {
        "data": data,
        "timestamp": timestamp,
        "proof": proof
    }
```

Python components can be used for blockchain processing, automation, analytics and integration with optimization models.

## C++ Implementation

The C++ implementation provides lower-level blockchain and cryptographic processing using OpenSSL-based components.

A simplified SHA-512 operation is:

```cpp
#include <openssl/sha.h>
#include <string>

std::string sha512(const std::string& data)
{
    unsigned char hash[SHA512_DIGEST_LENGTH];

    SHA512(
        reinterpret_cast<const unsigned char*>(data.c_str()),
        data.size(),
        hash
    );

    return std::string(
        reinterpret_cast<char*>(hash),
        SHA512_DIGEST_LENGTH
    );
}
```

The C++ components can be used for performance-sensitive blockchain processing and cryptographic operations.

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
