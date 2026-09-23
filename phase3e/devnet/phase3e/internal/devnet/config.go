package devnet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ValidatorConfig struct {
	MinBondUnits       int64  `json:"min_bond_units"`
	ActivationDelay    uint64 `json:"activation_delay"`
	UnbondingBlocks    uint64 `json:"unbonding_blocks"`
	JailBlocks         uint64 `json:"jail_blocks"`
	DoubleVoteSlashBPS uint32 `json:"double_vote_slash_bps"`
}

type NetworkConfig struct {
	ProposalTimeoutMS  int64  `json:"proposal_timeout_ms"`
	PrevoteTimeoutMS   int64  `json:"prevote_timeout_ms"`
	PrecommitTimeoutMS int64  `json:"precommit_timeout_ms"`
	RoundDeltaMS       int64  `json:"round_delta_ms"`
	MaxRound           uint64 `json:"max_round"`
	MaxMessageBytes    int    `json:"max_message_bytes"`
}

type NodePublic struct {
	Index             int      `json:"index"`
	NodeID            string   `json:"node_id"`
	NodeClass         string   `json:"node_class"`
	Roles             []string `json:"roles"`
	PeerID            string   `json:"peer_id"`
	ValidatorID       string   `json:"validator_id"`
	OperatorAddress   string   `json:"operator_address"`
	RewardAddress     string   `json:"reward_address"`
	ConsensusAddress  string   `json:"consensus_address"`
	IP                string   `json:"ip"`
	P2PPort           int      `json:"p2p_port"`
	RPCPort           int      `json:"rpc_port"`
	P2PMultiaddr      string   `json:"p2p_multiaddr"`
	Namespace         string   `json:"namespace,omitempty"`
	RootVeth          string   `json:"root_veth,omitempty"`
	NamespaceVeth     string   `json:"namespace_veth,omitempty"`
	TEPPubkey         string   `json:"tep_pubkey"`
	StorageRole       string   `json:"storage_role"`
	ConsensusProtocol string   `json:"consensus_protocol"`
}

type Manifest struct {
	SchemaVersion             int             `json:"schema_version"`
	Mode                      string          `json:"mode"`
	NodesCount                int             `json:"nodes_count"`
	ChainID                   uint64          `json:"chain_id"`
	ActivationHeight          uint64          `json:"activation_height"`
	LegacyPoWDifficulty       int             `json:"legacy_pow_difficulty"`
	PoHTicksPerBlock          int             `json:"poh_ticks_per_block"`
	ConsensusActivationHeight uint64          `json:"consensus_activation_height"`
	BootstrapHeight           int             `json:"bootstrap_height"`
	FeeCollector              string          `json:"fee_collector"`
	SlashCollector            string          `json:"slash_collector"`
	Validator                 ValidatorConfig `json:"validator"`
	ConsensusNetwork          NetworkConfig   `json:"consensus_network"`
	RecorderAddress           string          `json:"recorder_address"`
	Nodes                     []NodePublic    `json:"nodes"`
}

type NodeSecret struct {
	OperatorPrivateHex  string `json:"operator_private_hex"`
	RewardPrivateHex    string `json:"reward_private_hex"`
	ConsensusPrivateHex string `json:"consensus_private_hex"`
	P2PPrivateBase64    string `json:"p2p_private_base64"`
}

type RecorderSecret struct {
	PrivateHex string `json:"private_hex"`
}

func LoadJSON(path string, out interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func WriteJSON(path string, value interface{}, mode os.FileMode) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), mode)
}
