# HashBurst Blockchain

Go implementation of the HashBurst blockchain node — mainnet production release.

[![Go](https://img.shields.io/badge/Go-1.22-blue)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Network](https://img.shields.io/badge/Network-Mainnet%201337-orange)](https://blockchainapi.one/api/hashburst/health)
[![Node 5](https://img.shields.io/badge/Node%205-blockchainapi.one-brightgreen)](https://blockchainapi.one)

---

## Overview

HashBurst is a blockchain designed for federated DePIN (Decentralized Physical Infrastructure Network) resource sharing. The node implements:

- **APoW + PoH consensus** — fast blocks without network voting
- **Blockchain DNS** — nodes discover each other via the chain, no external DNS
- **HB-TEP integration** — direct encrypted node-to-node communication, Cloudflare-independent
- **Compact ledger** — ~700 bytes/block (gob binary), ~1000x smaller than Bitcoin

---

## Consensus Architecture

### Proof of History (PoH)

PoH is the cryptographic clock of each node. Instead of waiting for network consensus to establish block time, every node independently proves that time has passed by executing a sequential SHA-512 chain:

```
prev_poh → SHA-512 → SHA-512 → ... (400,000 times) → new_poh
```

This sequence is non-parallelizable — you cannot skip steps. It proves that approximately 40ms of CPU time has elapsed since the previous block, without requiring any coordination with other nodes.

Each block stores only the final PoH value (8 bytes). The intermediate sequence is discarded — this is why the ledger is compact.

### Adaptive Proof of Work (APoW)

APoW is a minimal validation layer on top of PoH. Difficulty = 4 means the block hash must begin with 4 zero hex characters (16 leading zero bits). On modern hardware this takes milliseconds, not minutes.

Combined result: blocks are produced every ~100–500ms, making HashBurst orders of magnitude faster than Bitcoin (10 minutes) or Ethereum (12 seconds).

---

## Fixes Included in This Release

### `blockchain/storage.go` — Disk Persistence

**Problem:** the blockchain existed only in RAM. Every restart meant losing the entire chain, which caused `NODE_REGISTRATION` to be re-sent on every boot since the blockchain DNS had no memory of previous registrations.

**Fix:** implemented `ChainStorage` using Go's stdlib `encoding/gob` — zero external dependencies. Each block is appended immediately after mining to two files:

| File             | Format                                       | Size/entry.      |
|------------------|----------------------------------------------|------------------|
| `blockchain.dat` | gob-encoded blocks with 4-byte length prefix | ~700 bytes/block |
| `blockchain.idx` | fixed-width index (blockNum, offset, size)   | 20 bytes/entry   |

On startup `LoadAll()` reads the chain sequentially from disk. For 1 million blocks with 2 transactions each: ~700 MB — compared to Bitcoin's ~600 GB at similar block count.

```go
// Every block is saved immediately after validation
if err := bc.storage.SaveBlock(newBlock); err != nil {
    log.Printf("Warning: persist block #%d: %v", newBlock.Index, err)
}
```

### `blockchain/mempool.go` — AddTransactionOnce

**Problem:** `GetTransactions()` returned all pending transactions but did not empty the mempool. The mining loop called `GetTransactions()` every 5 seconds, so the same transaction was included in multiple consecutive blocks, producing dozens of duplicate `NODE_REGISTRATION` entries in the chain.

**Fix 1:** `GetTransactions()` now atomically empties the mempool after returning the transactions, making double-inclusion structurally impossible:

```go
func (m *Mempool) GetTransactions() []*Transaction {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    txs := make([]*Transaction, 0, len(m.transactions))
    for _, tx := range m.transactions {
        txs = append(txs, tx)
    }
    m.transactions = make(map[string]*Transaction) // atomic clear
    return txs
}
```

**Fix 2:** added `AddTransactionOnce()` which checks for an existing transaction with the same `Sender + Receiver` combination before adding. Used exclusively for `NODE_REGISTRATION` to guarantee exactly-once semantics within a session:

```go
func (m *Mempool) AddTransactionOnce(tx *Transaction) bool {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    for _, existing := range m.transactions {
        if existing.Receiver == tx.Receiver && existing.Sender == tx.Sender {
            return false // already queued
        }
    }
    m.transactions[tx.ID] = tx
    return true
}
```

### `blockchain/blockchain.go` — Load from Disk on Startup

**Problem:** `NewBlockchain()` always created a fresh genesis block, ignoring any data previously saved on disk.

**Fix:** `NewBlockchain()` now calls `storage.Exists()` on startup. If `blockchain.dat` is found, `LoadAll()` restores the complete chain before the node begins mining. The genesis block is only created on true first boot:

```go
func newBlockchainWithDir(dir string) *Blockchain {
    storage := NewChainStorage(dir)
    bc := &Blockchain{MiningReward: 50.0, storage: storage}

    if storage.Exists() {
        blocks, err := storage.LoadAll()
        if err == nil && len(blocks) > 0 {
            bc.Blocks = blocks
            log.Printf("Blockchain loaded: %d blocks | %.4f MB",
                len(blocks), storage.Stats()["size_mb"].(float64))
            return bc
        }
    }

    // First boot — create genesis
    genesis := NewBlock([]*Transaction{}, "0", 0)
    bc.Blocks = []*Block{genesis}
    storage.SaveBlock(genesis)
    return bc
}
```

### `blockchain/registry.go` — BlockchainDNS

**Problem:** nodes discovered each other via a static `peers.json` file. If a server migrated to a new IP, every other node had to be manually updated. No automation, no resilience.

**Fix:** introduced the `NODE_REGISTRATION` transaction type. On first boot, every node writes a special transaction to the blockchain containing its full identity:

```go
type NodeRecord struct {
    NodeID      string   `json:"node_id"`      // "name-of-the-node"
    PeerID      string   `json:"peer_id"`      // "12D3KooW..."
    Multiaddrs  []string `json:"multiaddrs"`   // ["/ip4/<IP_ADDRESS>/tcp/30307/p2p/..."]
    TEPPubkey   string   `json:"tep_pubkey"`   // X25519 hex public key for HB-TEP
    TEPPort     int      `json:"tep_port"`     // 47777
    RPCEndpoint string   `json:"rpc_endpoint"` // "https://domain-name.tld/api/hashburst"
    ExternalIP  string   `json:"external_ip"`  // "<IP_ADDRESS>"
    Version     string   `json:"version"`
    Timestamp   int64    `json:"timestamp"`
    ChainID     int      `json:"chain_id"`     // 1337
}
```

`BlockchainDNS` scans all blocks for these transactions and exposes them via `/api/nodes`. This makes the blockchain a **decentralized DNS**:

- Cannot be censored — records are immutable once mined
- Cannot be spoofed — each record is associated with the node's wallet address
- No external dependency — no Cloudflare, no traditional DNS required
- Auto-updating — `tepPeerSyncLoop` writes `peers.json` from blockchain data every 60s

### `blockchain/p2p.go` — Persistent P2P Identity

**Problem:** `libp2p.New()` without an explicit identity option generates a new random Ed25519 key on every startup, producing a different Peer ID each time. This made the `NODE_REGISTRATION` record in the blockchain stale after every restart, and prevented other nodes from maintaining stable connections.

**Fix:** `NewP2PNode()` now accepts an optional `crypto.PrivKey` parameter. When provided, libp2p uses that key and the Peer ID is deterministic:

```go
func NewP2PNode(bc *Blockchain, mp *Mempool, port int, privKey crypto.PrivKey) (*P2PNode, error) {
    opts := []libp2p.Option{
        libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
        libp2p.ConnectionManager(cm),
    }
    if privKey != nil {
        opts = append(opts, libp2p.Identity(privKey)) // stable Peer ID
    }
    // ...
}
```

Also added `ConnectFromBlockchainDNS()` which reads bootstrap peers from the chain instead of hardcoded addresses:

```go
func (p *P2PNode) ConnectFromBlockchainDNS(ctx context.Context, excludeNodeID string) {
    addrs := p.DNS.GetP2PBootstrapAddrs(excludeNodeID)
    for _, addr := range addrs {
        p.Connect(ctx, addr)
    }
}
```

### `p2p_identity.go` — LoadOrCreateP2PIdentity

**Problem:** no mechanism existed to persist the libp2p key across restarts.

**Fix:** `LoadOrCreateP2PIdentity()` manages the Ed25519 key lifecycle:

1. On **first boot**: generates a new Ed25519 key pair, serializes it to JSON, saves to `/var/lib/hashburst/node_p2p.key` with `chmod 600`
2. On **subsequent boots**: reads and deserializes the existing key
3. In both cases: derives the Peer ID from the key and logs it

```go
func LoadOrCreateP2PIdentity(keyPath string) (*P2PIdentity, error) {
    if _, err := os.Stat(keyPath); os.IsNotExist(err) {
        // First boot — generate and save
        privKey, _, _ = crypto.GenerateEd25519Key(rand.Reader)
        // ... serialize and save to keyPath
    } else {
        // Load existing key
        // ... deserialize from keyPath
    }
    peerID, _ := peer.IDFromPrivateKey(privKey)
    return &P2PIdentity{PrivKey: privKey, PeerID: peerID}, nil
}
```

**Do not delete `node_p2p.key`** — it is the permanent identity of the node registered in the blockchain DNS. Deleting it forces a new `NODE_REGISTRATION` with a different Peer ID.

### `main.go` — NODE_REGISTRATION Triple Deduplication

**Problem:** `NODE_REGISTRATION` was being sent on every restart because the check was only in-memory. Since the blockchain was not persisted, the DNS was always empty on startup, so the registration check always returned "not found" and always re-queued the transaction.

**Fix:** three independent deduplication layers, each targeting a different failure scenario:

```
Layer 1 — Flag file on disk
    /var/lib/hashburst/node_registered.flag
    Written after successful mining. Survives restarts.
    → Prevents re-registration across restarts (primary guard)

Layer 2 — Blockchain DNS check
    bDNS.GetNode(nodeID) scans blocks loaded from disk
    → Catches cases where flag was lost but blockchain still has the record

Layer 3 — AddTransactionOnce in mempool
    Prevents duplicate queuing within the same session
    → Guards against race conditions during startup goroutine timing
```

Implementation in `main.go`:

```go
go func() {
    time.Sleep(5 * time.Second)

    // Layer 1: flag file
    flagFile := filepath.Join(storageDir, "node_registered.flag")
    if _, err := os.Stat(flagFile); err == nil {
        log.Printf("NODE_REGISTRATION: flag found — skipping")
        return
    }

    // Layer 2: blockchain DNS
    if existing, ok := bDNS.GetNode(nodeID); ok {
        log.Printf("NODE_REGISTRATION: found in blockchain — %s", existing.PeerID)
        os.WriteFile(flagFile, []byte(identity.PeerID.String()), 0644)
        return
    }

    // Layer 3: AddTransactionOnce
    tx := blockchain.NewNodeRegistration(rewardAddr, record)
    if mp.AddTransactionOnce(tx) {
        log.Printf("NODE_REGISTRATION queued | %s | %s", nodeID, identity.PeerID)
    }

    // Write flag after confirmed mining
    go func() {
        for i := 0; i < 30; i++ {
            time.Sleep(2 * time.Second)
            if _, ok := bDNS.GetNode(nodeID); ok {
                os.WriteFile(flagFile, []byte(identity.PeerID.String()), 0644)
                return
            }
        }
    }()
}()
```

---

## Persistence Files

All files are stored in `STORAGE_DIR` (default: `/var/lib/hashburst/`):

| File                   | Purpose                            | Delete safe?            |
|------------------------|------------------------------------|-------------------------|
| `blockchain.dat.     ` | All blocks in gob binary format    | Only for full reset     |
| `blockchain.idx`       | Block index (20 bytes/entry)       | Only for full reset     |
| `node_p2p.key`         | Ed25519 P2P identity               | **Never**               |
| `node_registered.flag` | NODE_REGISTRATION dedup flag       | Only for full reset     |
| `tep/peers.json`       | TEP peer cache from blockchain DNS | Safe — auto-regenerated |

### Full Reset Procedure (dev/test only)

```bash
systemctl stop hashburst-node
rm -f /var/lib/hashburst/blockchain.dat
rm -f /var/lib/hashburst/blockchain.idx
rm -f /var/lib/hashburst/node_registered.flag
# Do NOT delete node_p2p.key
systemctl start hashburst-node
```

---

## API Endpoints

| Method | Endpoint            | Description                                       |
|--------|---------------------|---------------------------------------------------|
| GET    | `/api/health`       | Node health: status, blockHeight, peerID, chainId |
| GET    | `/api/status`       | Full status: peers, tps, miner, version           |
| GET    | `/api/blocks`       | Last 10 blocks                                    |
| GET    | `/api/transactions` | All transactions with tx_type annotation          |
| GET.   | `/api/nodes`        | **Blockchain DNS** — all registered nodes         |
| GET    | `/api/tep/peers`    | TEP peer list for HB-TEP module                   |
| GET    | `/api/storage`      | Disk storage statistics                           |

### Example responses

```bash
# Node health (includes stable Peer ID)
curl https://blockchainapi.one/api/hashburst/health | jq
{
  "status": "ok",
  "blockHeight": 2,
  "node": "blockchainapi.one",
  "peerID": "12D3KooWCiH3B8E84UNsop5epp7vNXfC6oSg2iyB4wjyCm6a84ow",
  "chainId": 1337
}

# Blockchain DNS — registered nodes
curl https://blockchainapi.one/api/hashburst/api/nodes | jq
[
  {
    "node_id": "blockchainapi.one",
    "peer_id": "12D3KooWCiH3B8E84UNsop5epp7vNXfC6oSg2iyB4wjyCm6a84ow",
    "multiaddrs": ["/ip4/64.31.4.9/tcp/30307/p2p/12D3KooW..."],
    "tep_pubkey": "50506353bd0ac23aec8502e3d5ed6c018975a7c5ea6e22dc363df321d6ca8960",
    "tep_port": 47777,
    "rpc_endpoint": "https://blockchainapi.one/api/hashburst",
    "external_ip": "64.31.4.9"
  }
]

# Storage statistics
curl http://localhost:8009/api/storage | jq
{
  "exists": true,
  "blocks": 2,
  "size_bytes": 1469,
  "size_mb": 0.0014,
  "dir": "/var/lib/hashburst"
}
```

---

## Environment Variables

All read from `/etc/hashburst/env` via systemd `EnvironmentFile`:

```bash
NODE_ID=blockchainapi.one           # Unique node identifier
EXTERNAL_IP=64.31.4.9               # Public IP (written in NODE_REGISTRATION)
RPC_PORT=8009                        # HTTP RPC port
P2P_PORT=30307                       # libp2p TCP port
REWARD_ADDRESS=0xYOUR_ADDRESS        # Wallet for mining rewards (HBT)
P2P_KEY_PATH=/var/lib/hashburst/node_p2p.key  # Ed25519 identity key path
STORAGE_DIR=/var/lib/hashburst       # Blockchain persistence directory
RPC_ENDPOINT=https://yourdomain.com/api/hashburst  # Public RPC URL
TEP_PUBKEY=hex...                    # X25519 pubkey for HB-TEP (set after first boot)
BOOTSTRAP_PEERS=/ip4/64.31.4.9/tcp/30307/p2p/12D3KooW...  # Comma-separated multiaddrs
```

---

## Build

```bash
cd GO
go mod tidy
go build -o hashburst-node .

# Run (reads /etc/hashburst/env automatically via systemd,
#      or export variables manually for testing)
export NODE_ID=testnode
export STORAGE_DIR=/tmp/hashburst-test
./hashburst-node
```

---

## Mainnet Nodes

| Node   | Domain            | IP            | P2P Multiaddr                                       |
|--------|-------------------|---------------|-----------------------------------------------------|
| Node 5 | domain-name.tld   | <IP_ADDRESS>. | `/ip4/<IP_ADDRESS>/tcp/30307/p2p/12D3Ko...a84ow`    |
| Node 4 | hashburst.io      | 77.90.188.157 | `/ip4/77.90.188.157/tcp/30306/p2p/QmHashBurstNode4` |

---

## Related Repositories

| Repository                                                                                                  | Description                        |
|-------------------------------------------------------------------------------------------------------------|------------------------------------|
| [hashburst/blockchain-hvm-framework](https://github.com/hashburst/blockchain-hvm-framework)                 | HBT-20 / HBT-721 smart contract VM |
| [hashburst/node-installer](https://github.com/hashburst/node-installer)                                     | Node installer for Ubuntu 24.04    |
| [hashburst/HashBurst-Blockchain-WebApp-Core](https://github.com/hashburst/HashBurst-Blockchain-WebApp-Core) | Web application and block explorer |
| [hashburst/HPC-Cryptominer-Open-Source](https://github.com/hashburst/HPC-Cryptominer-Open-Source)           | HPC miner for HashBurst network    |

---

## License

MIT — see [LICENSE](LICENSE)
