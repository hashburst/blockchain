// Package bootstrap prepares a NEW testnet offline. It never starts a node,
// imports a devnet identity or writes to an existing destination.
package bootstrap

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"hashburst/blockchain"
	"hashburst/consensus"
	"hashburst/internal/testnet"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

const compute = uint64(80000)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type Identity struct {
	Registration *blockchain.Transaction   `json:"registration"`
	Validator    *protocolv2.TransactionV2 `json:"validator_registration"`
}
type Options struct {
	ChainID                  uint64
	NodeID, IP, TEPPublicKey string
	P2PPort                  int
}

func CheckChain(id uint64) error {
	if id == 0 || id == 1337 || id > math.MaxInt64 {
		return fmt.Errorf("explicit non-legacy chain ID in 1..MaxInt64 required; 1337 is reserved for legacy")
	}
	return nil
}
func Protocol(id uint64, n int) blockchain.ProtocolV2Config {
	c := blockchain.DefaultProtocolV2Config()
	c.ChainID = id
	c.ActivationHeight = uint64(n + 2)
	c.ConsensusActivationHeight = uint64(n + 4)
	c.LegacyPoWDifficulty = 1
	c.PoHTicksPerBlock = 4000
	c.Validator.MinBondUnits = 10 * blockchain.AmountScale
	c.Validator.ActivationDelay = 1
	c.Validator.UnbondingBlocks = 8
	c.Validator.JailBlocks = 100
	c.ConsensusNetwork.ProposalTimeout = 3 * time.Second
	c.ConsensusNetwork.PrevoteTimeout = 2 * time.Second
	c.ConsensusNetwork.PrecommitTimeout = 2 * time.Second
	c.ConsensusNetwork.RoundTimeoutDelta = 500 * time.Millisecond
	c.ConsensusNetwork.MaxFutureHeight = 4
	return c
}
func newDir(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("absolute clean destination required")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil {
		return e
	}
	if parent != filepath.Dir(path) {
		return fmt.Errorf("symlink parent refused")
	}
	if path == "/var/lib/hashburst" || strings.HasPrefix(path, "/var/lib/hashburst/") {
		return fmt.Errorf("public state path refused")
	}
	return os.Mkdir(path, 0700) // EEXIST is intentional; no overwrite or recursive cleanup.
}
func write(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func jsonFile(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return write(path, append(b, '\n'))
}

// Generate runs ON THE VALIDATOR HOST. Only public.json leaves this directory.
// TEPPublicKey must be the real public X25519 identity of that host's TEP node.
func Generate(out string, o Options) error {
	if e := CheckChain(o.ChainID); e != nil {
		return e
	}
	ip := net.ParseIP(o.IP)
	if !safeID.MatchString(o.NodeID) || ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || o.P2PPort < 1024 || o.P2PPort > 65535 {
		return fmt.Errorf("valid node ID, IPv4 and unprivileged P2P port required")
	}
	tep, e := hex.DecodeString(o.TEPPublicKey)
	if e != nil || len(tep) != 32 || strings.Trim(o.TEPPublicKey, "0") == "" {
		return fmt.Errorf("real 32-byte TEP public key required")
	}
	if e = newDir(out); e != nil {
		return e
	}
	op, e := wallet.NewWallet()
	if e != nil {
		return e
	}
	reward, e := wallet.NewWallet()
	if e != nil {
		return e
	}
	signer, e := wallet.NewWallet()
	if e != nil {
		return e
	}
	key, _, e := crypto.GenerateEd25519Key(rand.Reader)
	if e != nil {
		return e
	}
	pid, e := peer.IDFromPrivateKey(key)
	if e != nil {
		return e
	}
	raw, e := crypto.MarshalPrivateKey(key)
	if e != nil {
		return e
	}
	for name, value := range map[string]string{"p2p.key": base64.StdEncoding.EncodeToString(raw), "consensus.key": hex.EncodeToString(signer.PrivateKeyBytes()), "operator.key": hex.EncodeToString(op.PrivateKeyBytes()), "reward.key": hex.EncodeToString(reward.PrivateKeyBytes())} {
		if e = write(filepath.Join(out, name), []byte(value+"\n")); e != nil {
			return e
		}
	}
	addr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", o.IP, o.P2PPort, pid)
	record := blockchain.NodeRecordV2{NodeRecord: blockchain.NodeRecord{NodeID: o.NodeID, PeerID: pid.String(), Multiaddrs: []string{addr}, TEPPubkey: strings.ToLower(o.TEPPublicKey), TEPPort: 47777, ExternalIP: o.IP, Version: "hvm-testnet-bootstrap-v1", ChainID: int(o.ChainID)}, NodeClass: blockchain.NodeClassNode, Roles: []blockchain.NodeRole{blockchain.NodeRoleFull}, TEPEnabled: true, Capabilities: []string{blockchain.NodeCapabilityHVMV2, blockchain.NodeCapabilityValidatorV2, blockchain.NodeCapabilityConsensusV2}, ProtocolVersion: blockchain.BlockVersionV2}
	registration := blockchain.NewNodeRegistrationV2(op.Address(), record)
	if e = registration.Sign(op); e != nil {
		return e
	}
	c := Protocol(o.ChainID, 4)
	data, e := json.Marshal(consensus.RegisterRequest{NodeID: o.NodeID, PeerID: pid.String(), ConsensusPubKey: signer.PublicKeyHexCompressed(), RewardAddress: reward.Address(), BondUnits: c.Validator.MinBondUnits})
	if e != nil {
		return e
	}
	fee, e := c.FeePolicy.ComputeFee(compute)
	if e != nil {
		return e
	}
	tx := protocolv2.NewTransactionV2(c.ChainID, protocolv2.TxValidatorRegister, op.Address(), "", c.Validator.MinBondUnits, 0, compute, fee, data)
	if e = tx.Sign(op); e != nil {
		return e
	}
	return jsonFile(filepath.Join(out, "public.json"), Identity{registration, tx})
}

func validate(id uint64, v Identity) (blockchain.NodeRecordV2, consensus.RegisterRequest, error) {
	var r blockchain.NodeRecordV2
	var request consensus.RegisterRequest
	fail := func(s string) (blockchain.NodeRecordV2, consensus.RegisterRequest, error) {
		return r, request, fmt.Errorf("invalid signed identity: %s", s)
	}
	if v.Registration == nil || v.Validator == nil {
		return fail("missing transaction")
	}
	if e := v.Registration.Verify(); e != nil {
		return fail(e.Error())
	}
	if e := v.Validator.Verify(id); e != nil {
		return fail(e.Error())
	}
	if e := json.Unmarshal([]byte(v.Registration.Data), &r); e != nil {
		return fail(e.Error())
	}
	if e := r.ValidateForValidator(id); e != nil {
		return fail(e.Error())
	}
	if e := json.Unmarshal(v.Validator.Data, &request); e != nil {
		return fail(e.Error())
	}
	if !safeID.MatchString(r.NodeID) || !wallet.IsValidAddress(v.Registration.Sender) || v.Registration.Receiver != blockchain.RegistryAddress || v.Registration.Amount != 0 || v.Validator.Type != protocolv2.TxValidatorRegister || v.Validator.Sender != v.Registration.Sender || request.NodeID != r.NodeID || request.PeerID != r.PeerID {
		return fail("identity binding")
	}
	c := Protocol(id, 4)
	fee, _ := c.FeePolicy.ComputeFee(compute)
	if request.BondUnits != c.Validator.MinBondUnits || v.Validator.ValueUnits != request.BondUnits || v.Validator.Sequence != 0 || v.Validator.ComputeLimit != compute || v.Validator.MaxFeeUnits != fee || v.Validator.To != "" {
		return fail("validator parameters")
	}
	ip := net.ParseIP(r.ExternalIP)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return fail("IPv4")
	}
	if len(r.Multiaddrs) != 1 {
		return fail("one P2P address required")
	}
	tep, e := hex.DecodeString(r.TEPPubkey)
	if e != nil || len(tep) != 32 || strings.Trim(r.TEPPubkey, "0") == "" {
		return fail("TEP key")
	}
	return r, request, nil
}

// Assemble accepts only public signed identities; it never receives signing keys.
// Output contains a public checkpoint and per-node observer/validator configs.
func Assemble(out string, id uint64, identities []Identity) error {
	if e := CheckChain(id); e != nil {
		return e
	}
	n := len(identities)
	if n < 4 || n > 6 {
		return fmt.Errorf("4..6 validator identities required")
	}
	records := make([]blockchain.NodeRecordV2, n)
	requests := make([]consensus.RegisterRequest, n)
	seen := map[string]bool{}
	for i, v := range identities {
		r, q, e := validate(id, v)
		if e != nil {
			return e
		}
		records[i] = r
		requests[i] = q
		for _, s := range []string{"node:" + r.NodeID, "peer:" + r.PeerID, "key:" + strings.ToLower(q.ConsensusPubKey), "operator:" + strings.ToLower(v.Registration.Sender), "tep:" + strings.ToLower(r.TEPPubkey), "endpoint:" + strings.Split(r.Multiaddrs[0], "/p2p/")[0]} {
			if seen[s] {
				return fmt.Errorf("duplicate identity %s", s)
			}
			seen[s] = true
		}
	}
	// Validate all runtime shapes before creating any chain state.
	cfg := Protocol(id, n)
	configs := make([]testnet.Config, n)
	for i, r := range records {
		vid, e := consensus.ValidatorID(requests[i].ConsensusPubKey)
		if e != nil {
			return e
		}
		prefix := fmt.Sprintf("/ip4/%s/tcp/", r.ExternalIP)
		suffix := "/p2p/" + r.PeerID
		if !strings.HasPrefix(r.Multiaddrs[0], prefix) || !strings.HasSuffix(r.Multiaddrs[0], suffix) {
			return fmt.Errorf("P2P address binding mismatch")
		}
		var port int
		portstr := strings.TrimSuffix(strings.TrimPrefix(r.Multiaddrs[0], prefix), suffix)
		if _, e = fmt.Sscanf(portstr, "%d", &port); e != nil || fmt.Sprint(port) != portstr {
			return fmt.Errorf("invalid P2P port")
		}
		c := testnet.Config{Schema: 1, Network: "testnet", NodeID: r.NodeID, Role: "observer", DataDir: "/var/lib/hashburst-hvm-testnet", RPCListen: "127.0.0.1:18009", P2PListenIP: "0.0.0.0", P2PPort: port, PeerID: r.PeerID, P2PKeyFile: "/etc/hashburst-hvm-testnet/p2p.key", ValidatorID: vid, GenesisHash: strings.Repeat("0", 64), CheckpointHash: strings.Repeat("0", 64), Protocol: cfg}
		for j, other := range records {
			if i != j {
				c.Bootnodes = append(c.Bootnodes, other.Multiaddrs[0])
			}
		}
		if e = c.Validate(); e != nil {
			return e
		}
		configs[i] = c
	}
	if e := newDir(out); e != nil {
		return e
	}
	state := filepath.Join(out, "checkpoint")
	bc := blockchain.NewBlockchainWithDirAndV2Config(state, cfg)
	for _, v := range identities {
		bc.SetPendingTransactions([]*blockchain.Transaction{v.Registration}, nil)
		if e := bc.AddBlock(v.Registration.Sender); e != nil {
			return e
		}
	}
	bc.SetPendingTransactions(nil, nil)
	if e := bc.AddBlock(identities[0].Registration.Sender); e != nil {
		return e
	}
	txs := make([]*protocolv2.TransactionV2, n)
	for i, v := range identities {
		txs[i] = v.Validator
	}
	bc.SetPendingTransactions(nil, txs)
	if e := bc.AddBlock(identities[0].Registration.Sender); e != nil {
		return e
	}
	bc.SetPendingTransactions(nil, nil)
	if e := bc.AddBlock(identities[0].Registration.Sender); e != nil {
		return e
	}
	height := bc.Height()
	if uint64(height+1) != cfg.ConsensusActivationHeight {
		return fmt.Errorf("unexpected checkpoint height")
	}
	set := bc.CurrentValidatorSet(uint64(height + 1))
	if len(set.Validators) != n {
		return fmt.Errorf("validator set mismatch")
	}
	blocks, e := blockchain.NewChainStorage(state).LoadAll()
	if e != nil {
		return e
	}
	for i, c := range configs {
		c.GenesisHash = blocks[0].Hash
		c.CheckpointHeight = height
		c.CheckpointHash = blocks[height].Hash
		if e = jsonFile(filepath.Join(out, c.NodeID+".observer.json"), c); e != nil {
			return e
		}
		c.Role = "validator"
		c.ConsensusKeyFile = "/etc/hashburst-hvm-testnet/consensus.key"
		if e = jsonFile(filepath.Join(out, c.NodeID+".validator.json"), c); e != nil {
			return e
		}
		configs[i] = c
	}
	// Genesis implementation is inherited, never modified. Network separation is
	// pinned by chain ID, protocol digest and this validator-specific checkpoint.
	if e = jsonFile(filepath.Join(out, "network.json"), map[string]any{"schema": 1, "network": "testnet", "chain_id": id, "genesis_hash": blocks[0].Hash, "checkpoint_height": height, "checkpoint_hash": blocks[height].Hash, "config_digest": configs[0].Pin(), "validator_set": set, "nodes": records}); e != nil {
		return e
	}
	if e = write(filepath.Join(out, "BOOTSTRAP_COMPLETE"), []byte(configs[0].Pin()+"\n")); e != nil {
		return e
	}
	var lines []string
	e = filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(out, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		lines = append(lines, fmt.Sprintf("%x  %s", sum, rel))
		return nil
	})
	if e != nil {
		return e
	}
	sort.Strings(lines)
	return write(filepath.Join(out, "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"))
}
