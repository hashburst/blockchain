package blockchain

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// NODE_REGISTRATION — transazione speciale che ogni nodo invia alla blockchain
// alla prima accensione (o ad ogni riavvio se l'ID è cambiato).
//
// Questo rende la blockchain un DNS distribuito e decentralizzato:
//   NodeID  →  multiaddr P2P + TEP pubkey + IP + porte
//
// Non può essere falsificato senza controllare la chiave privata del nodo.
// Non dipende da DNS, Cloudflare o qualsiasi intermediario.
// ────────────────────────────────────────────────────────────────────────────

const (
	TxTypeNodeRegistration   = "NODE_REGISTRATION"
	TxTypeNodeRegistrationV2 = "NODE_REGISTRATION_V2"

	NodeRecordVersion2 = 2
)

type NodeClass string
type NodeRole string

const (
	NodeClassNode NodeClass = "node"
	NodeClassHPC  NodeClass = "hpc"
)

const (
	NodeRoleFull       NodeRole = "full"
	NodeRoleBlockchain NodeRole = "blockchain"
	NodeRoleStorage    NodeRole = "storage"
	NodeRoleEdge       NodeRole = "edge"
)

const (
	NodeCapabilityHVMV2       = "hvm-v2"
	NodeCapabilityValidatorV2 = "validator-v2"
	NodeCapabilityConsensusV2 = "consensus-v2"
)

// NodeRecord è il payload della transazione NODE_REGISTRATION.
// Viene serializzato in JSON e incluso nel campo Data della transazione.
type NodeRecord struct {
	NodeID      string   `json:"node_id"`      // es. "node5-blockchainapi-one"
	PeerID      string   `json:"peer_id"`      // libp2p peer ID (es. 12D3KooW...)
	Multiaddrs  []string `json:"multiaddrs"`   // es. ["/ip4/64.31.4.9/tcp/30307/p2p/..."]
	TEPPubkey   string   `json:"tep_pubkey"`   // X25519 hex pubkey per HB-TEP
	TEPPort     int      `json:"tep_port"`     // porta UDP TEP (default 47777)
	RPCEndpoint string   `json:"rpc_endpoint"` // es. "https://blockchainapi.one/api/hashburst"
	ExternalIP  string   `json:"external_ip"`  // IP pubblico del nodo
	Version     string   `json:"version"`      // versione software nodo
	Timestamp   int64    `json:"timestamp"`    // unix timestamp registrazione
	ChainID     int      `json:"chain_id"`     // 1337 per mainnet
}

// NodeRegistrationTx è la transazione completa NODE_REGISTRATION

// NodeRecordV2 extends the discovery record with the participant class plus
// explicit operational roles and execution/consensus capabilities. NodeClass
// may be provisioned from authenticated node/HPC metadata (including the
// stakeholder source used by deployment tooling), but validator admission is
// based on this confirmed on-chain V2 record rather than a live list.json lookup.
type NodeRecordV2 struct {
	NodeRecord
	RecordVersion int       `json:"record_version"`
	NodeClass     NodeClass `json:"node_class"`
	// Role is retained as a compatibility alias for early Phase 3C/3D fixtures.
	// New registrations should use Roles because one physical/virtual node can
	// simultaneously be a full blockchain participant and an edge/storage node.
	Role            NodeRole   `json:"role,omitempty"`
	Roles           []NodeRole `json:"roles,omitempty"`
	TEPEnabled      bool       `json:"tep_enabled"`
	Capabilities    []string   `json:"capabilities"`
	ProtocolVersion uint16     `json:"protocol_version"`
}

func (r NodeRecordV2) effectiveRoles() []NodeRole {
	out := make([]NodeRole, 0, len(r.Roles)+1)
	seen := make(map[NodeRole]struct{})
	for _, role := range r.Roles {
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if r.Role != "" {
		if _, ok := seen[r.Role]; !ok {
			out = append(out, r.Role)
		}
	}
	return out
}

func (r NodeRecordV2) HasRole(role NodeRole) bool {
	for _, candidate := range r.effectiveRoles() {
		if candidate == role {
			return true
		}
	}
	return false
}

func (r NodeRecordV2) ValidateForValidator(expectedChainID uint64) error {
	if r.RecordVersion != NodeRecordVersion2 {
		return fmt.Errorf("node record version %d: expected %d", r.RecordVersion, NodeRecordVersion2)
	}
	if r.ChainID <= 0 || uint64(r.ChainID) != expectedChainID {
		return fmt.Errorf("node record chain_id %d does not match protocol chain_id %d", r.ChainID, expectedChainID)
	}
	if r.NodeClass != NodeClassNode && r.NodeClass != NodeClassHPC {
		return fmt.Errorf("node class %q is not supported", r.NodeClass)
	}
	if !r.HasRole(NodeRoleFull) && !r.HasRole(NodeRoleBlockchain) {
		return fmt.Errorf("node roles %v do not include validator-eligible full/blockchain", r.effectiveRoles())
	}
	if !r.TEPEnabled || strings.TrimSpace(r.TEPPubkey) == "" {
		return fmt.Errorf("validator node requires active TEP identity")
	}
	if r.ProtocolVersion < BlockVersionV2 {
		return fmt.Errorf("node protocol version %d is below required %d", r.ProtocolVersion, BlockVersionV2)
	}
	required := map[string]bool{NodeCapabilityHVMV2: false, NodeCapabilityValidatorV2: false, NodeCapabilityConsensusV2: false}
	for _, c := range r.Capabilities {
		c = strings.ToLower(strings.TrimSpace(c))
		if _, ok := required[c]; ok {
			required[c] = true
		}
	}
	for c, ok := range required {
		if !ok {
			return fmt.Errorf("node capability %q required for validator admission", c)
		}
	}
	return nil
}

func NewNodeRegistrationV2(nodeWalletAddress string, record NodeRecordV2) *Transaction {
	record.RecordVersion = NodeRecordVersion2
	record.Timestamp = time.Now().Unix()
	data, err := json.Marshal(record)
	if err != nil {
		log.Fatalf("NODE_REGISTRATION_V2: failed to marshal record: %v", err)
	}
	return NewDataTransaction(nodeWalletAddress, RegistryAddress, 0, string(data))
}

type NodeRegistrationTx struct {
	Transaction
	TxType string     `json:"tx_type"` // sempre "NODE_REGISTRATION"
	Record NodeRecord `json:"record"`
}

// NewNodeRegistration crea una transazione di registrazione nodo.
// Il campo Sender è l'indirizzo wallet del nodo (per autenticare l'origine).
// Il campo Data contiene il NodeRecord serializzato.
func NewNodeRegistration(nodeWalletAddress string, record NodeRecord) *Transaction {
	record.Timestamp = time.Now().Unix()

	data, err := json.Marshal(record)
	if err != nil {
		log.Fatalf("NODE_REGISTRATION: failed to marshal record: %v", err)
	}

	// La transazione usa il formato standard ma con Sender=nodeAddress,
	// Receiver=indirizzo speciale registry, Amount=0 (non trasferisce fondi)
	tx := &Transaction{
		Sender:   nodeWalletAddress,
		Receiver: "0x0000000000000000000000000000000000REGISTRY",
		Amount:   0,
	}
	// Il campo ID viene calcolato su tutto il payload incluso il record
	tx.ID = fmt.Sprintf("NODE_REG_%s_%d", record.NodeID, record.Timestamp)

	// Usiamo Signature per portare il JSON del record (estensione del protocollo)
	tx.Signature = string(data)
	//return tx
	return NewDataTransaction(nodeWalletAddress, RegistryAddress, 0, string(data))
}

// ────────────────────────────────────────────────────────────────────────────
// BlockchainDNS — legge i record NODE_REGISTRATION dalla blockchain
// e li restituisce come mappa NodeID → NodeRecord
// ────────────────────────────────────────────────────────────────────────────

// BlockchainDNS implementa la discovery dei peer dalla blockchain
// senza dipendere da DNS, Cloudflare o file statici.
type BlockchainDNS struct {
	bc *Blockchain
}

// NewBlockchainDNS crea un nuovo resolver DNS basato sulla blockchain
func NewBlockchainDNS(bc *Blockchain) *BlockchainDNS {
	return &BlockchainDNS{bc: bc}
}

// GetAllNodes restituisce tutti i nodi registrati nella blockchain.
// In caso di registrazioni multiple dello stesso NodeID, vince l'ultima.
func (d *BlockchainDNS) GetAllNodes() map[string]NodeRecord {
	nodes := make(map[string]NodeRecord)

	for _, block := range d.bc.Blocks {
		for _, tx := range block.Transactions {
			// Identifica le transazioni NODE_REGISTRATION
			if tx.Receiver != RegistryAddress {
				continue
			}
			if tx.Data == "" {
				continue
			}
			var record NodeRecord
			if err := json.Unmarshal([]byte(tx.Data), &record); err != nil {
				continue
			}
			if record.NodeID == "" || record.PeerID == "" {
				continue
			}
			// L'ultima registrazione sovrascrive le precedenti (update)
			nodes[record.NodeID] = record
		}
	}

	return nodes
}

// GetNode restituisce il record di un nodo specifico
func (d *BlockchainDNS) GetNode(nodeID string) (NodeRecord, bool) {
	nodes := d.GetAllNodes()
	rec, ok := nodes[nodeID]
	return rec, ok
}

// GetTEPPeers restituisce la lista dei peer TEP da usare per la connessione
// diretta nodo-nodo, senza DNS e senza Cloudflare.
func (d *BlockchainDNS) GetTEPPeers(excludeNodeID string) []TEPPeerEntry {
	peers := []TEPPeerEntry{}
	for nodeID, record := range d.GetAllNodes() {
		if nodeID == excludeNodeID {
			continue // non aggiungere se stessi
		}
		if record.ExternalIP == "" || record.TEPPubkey == "" {
			continue
		}
		peers = append(peers, TEPPeerEntry{
			ID:     nodeID,
			IP:     record.ExternalIP,
			Port:   record.TEPPort,
			Pubkey: record.TEPPubkey,
			PeerID: record.PeerID,
		})
	}
	return peers
}

// GetP2PBootstrapAddrs restituisce gli indirizzi libp2p per il bootstrap P2P
func (d *BlockchainDNS) GetP2PBootstrapAddrs(excludeNodeID string) []string {
	addrs := []string{}
	for nodeID, record := range d.GetAllNodes() {
		if nodeID == excludeNodeID {
			continue
		}
		addrs = append(addrs, record.Multiaddrs...)
	}
	return addrs
}

// TEPPeerEntry è il formato peer per il modulo HB-TEP
type TEPPeerEntry struct {
	ID     string `json:"id"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Pubkey string `json:"pubkey"`
	PeerID string `json:"peer_id"`
}
