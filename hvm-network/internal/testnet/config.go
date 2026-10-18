package testnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"hashburst/blockchain"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Schema           int                         `json:"schema"`
	Network          string                      `json:"network"`
	NodeID           string                      `json:"node_id"`
	Role             string                      `json:"role"`
	DataDir          string                      `json:"data_dir"`
	RPCListen        string                      `json:"rpc_listen"`
	P2PListenIP      string                      `json:"p2p_listen_ip"`
	P2PPort          int                         `json:"p2p_port"`
	PeerID           string                      `json:"peer_id"`
	P2PKeyFile       string                      `json:"p2p_key_file"`
	ValidatorID      string                      `json:"validator_id"`
	ConsensusKeyFile string                      `json:"consensus_key_file"`
	GenesisHash      string                      `json:"genesis_hash"`
	CheckpointHeight int                         `json:"checkpoint_height"`
	CheckpointHash   string                      `json:"checkpoint_hash"`
	Bootnodes        []string                    `json:"bootnodes"`
	Protocol         blockchain.ProtocolV2Config `json:"protocol"`
}

func Load(path string) (Config, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return c, fmt.Errorf("trailing configuration data")
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Network == "testnet" && c.Protocol.EVM != nil && c.Protocol.ChainID != 4735490 {
		return fmt.Errorf("testnet EVM requires chain ID 4735490; mainnet uses a separate runtime configuration")
	}
	if c.Network == "mainnet" {
		if err := c.validateMainnetProfile(); err != nil {
			return err
		}
	}
	if c.P2PKeyFile == "" {
		return fmt.Errorf("P2P key path required")
	}
	if c.Schema != 1 || (c.Network != "testnet" && c.Network != "mainnet") || strings.TrimSpace(c.NodeID) == "" {
		return fmt.Errorf("explicit schema 1, testnet/mainnet and node_id required")
	}
	if c.Role != "observer" && c.Role != "validator" {
		return fmt.Errorf("role must be observer or validator")
	}
	if c.Role == "validator" && (c.ValidatorID == "" || c.ConsensusKeyFile == "") {
		return fmt.Errorf("validator identity and key required")
	}
	if c.Role == "observer" && c.ConsensusKeyFile != "" {
		return fmt.Errorf("observer must not load signing key")
	}
	host, port, e := net.SplitHostPort(c.RPCListen)
	if e != nil {
		return e
	}
	pn, pe := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || pe != nil || pn < 1024 || pn > 65535 {
		return fmt.Errorf("RPC must bind a fixed loopback endpoint")
	}
	if net.ParseIP(c.P2PListenIP) == nil || c.P2PPort < 1024 || c.P2PPort > 65535 {
		return fmt.Errorf("explicit P2P IP and unprivileged port required")
	}
	if _, e := peer.Decode(c.PeerID); e != nil {
		return e
	}
	for _, h := range []string{c.GenesisHash, c.CheckpointHash} {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 32 {
			return fmt.Errorf("32-byte genesis and checkpoint hashes required")
		}
	}
	if c.CheckpointHeight < 0 {
		return fmt.Errorf("invalid checkpoint")
	}
	if !filepath.IsAbs(c.DataDir) || filepath.Clean(c.DataDir) != c.DataDir || c.DataDir == "/" || c.DataDir == "/var/lib/hashburst" || strings.HasPrefix(c.DataDir, "/var/lib/hashburst/") {
		return fmt.Errorf("separate absolute testnet state directory required")
	}
	for _, p := range []string{c.P2PKeyFile, c.ConsensusKeyFile} {
		if p != "" && !filepath.IsAbs(p) {
			return fmt.Errorf("absolute key paths required")
		}
	}
	if len(c.Bootnodes) == 0 {
		return fmt.Errorf("explicit bootnodes required")
	}
	for _, b := range c.Bootnodes {
		a, e := ma.NewMultiaddr(b)
		if e != nil {
			return e
		}
		if _, e = peer.AddrInfoFromP2pAddr(a); e != nil {
			return e
		}
	}
	if c.Protocol.ChainID > math.MaxInt64 {
		return fmt.Errorf("chain ID exceeds RPC representation")
	}
	return c.Protocol.Validate()
}
func (c Config) Pin() string {
	// Shared digest covers consensus parameters; identity binding is added in the disk pin.
	b, _ := json.Marshal(struct {
		Network    string
		Genesis    string
		Height     int
		Checkpoint string
		Protocol   blockchain.ProtocolV2Config
	}{c.Network, c.GenesisHash, c.CheckpointHeight, c.CheckpointHash, c.Protocol})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Mainnet has a separate on-disk namespace and listeners. Existing testnet
// pins cannot be relabelled: the pin covers Network, ChainID and the checkpoint.
func (c Config) validateMainnetProfile() error {
	if c.Protocol.GenesisImport == nil {
		return fmt.Errorf("mainnet requires the approved genesis import commitment")
	}
	bootstrapBoundary := c.Protocol.MainnetBootstrapEnd != 0 && c.CheckpointHeight > 0 &&
		uint64(c.CheckpointHeight) == c.Protocol.MainnetBootstrapEnd &&
		c.Protocol.ConsensusActivationHeight == c.Protocol.MainnetBootstrapEnd+1
	if c.CheckpointHeight < 1 || (!bootstrapBoundary && !c.Protocol.ConsensusEnabledAt(c.CheckpointHeight)) {
		return fmt.Errorf("economic genesis alone is not an activatable validator checkpoint")
	}

	if c.Protocol.ChainID != 4735489 || c.Protocol.EVM == nil {
		return fmt.Errorf("mainnet requires chain ID 4735489 and explicit EVM configuration")
	}
	if c.DataDir != "/var/lib/hashburst-hvm-mainnet" && c.DataDir != "/var/lib/hashburst-hvm-mainnet-ingress" {
		return fmt.Errorf("mainnet requires its dedicated state directory")
	}
	keyRoot := "/etc/hashburst-hvm-mainnet/"
	if c.Role == "observer" {
		keyRoot = "/etc/hashburst-hvm-mainnet-ingress/"
	}
	for _, p := range []string{c.P2PKeyFile, c.ConsensusKeyFile} {
		if p != "" && (filepath.Clean(p) != p || !strings.HasPrefix(p, keyRoot)) {
			return fmt.Errorf("mainnet requires separate canonical key paths")
		}
	}
	if (c.Role == "observer") != (c.DataDir == "/var/lib/hashburst-hvm-mainnet-ingress") {
		return fmt.Errorf("mainnet role/state directory mismatch")
	}
	if c.RPCListen != "127.0.0.1:18019" || c.P2PPort != 31317 {
		return fmt.Errorf("mainnet requires dedicated RPC 18019 and P2P 31317")
	}
	return nil
}
