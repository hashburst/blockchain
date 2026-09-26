package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hashburst/blockchain"
	"hashburst/consensus"
	hvm_network "hashburst/devnet/hvm-network/internal/devnet"
	"hashburst/protocolv2"
	"hashburst/wallet"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const validatorRegisterCompute = uint64(80_000)

type validatorFixture struct {
	pub       hvm_network.NodePublic
	secret    hvm_network.NodeSecret
	operator  *wallet.Wallet
	reward    *wallet.Wallet
	consensus *wallet.Wallet
}

func main() {
	out := flag.String("out", "./devnet/hvm-network/run", "output directory")
	nodesN := flag.Int("nodes", 4, "validator node count (4-6)")
	mode := flag.String("mode", "loopback", "loopback or netns")
	flag.Parse()
	if *nodesN < 4 || *nodesN > 6 {
		fatalf("nodes must be 4-6")
	}
	if *mode != "loopback" && *mode != "netns" {
		fatalf("mode must be loopback or netns")
	}
	absOut, err := filepath.Abs(*out)
	must(err)
	safeMarker := string(os.PathSeparator) + "devnet" + string(os.PathSeparator) + "hvm-network" + string(os.PathSeparator)
	if !strings.Contains(absOut, safeMarker) || !strings.HasPrefix(filepath.Base(absOut), "run-") {
		fatalf("refusing unsafe --out %s: expected .../devnet/hvm-network/run-*", absOut)
	}
	if err := os.RemoveAll(absOut); err != nil {
		fatalf("remove old out: %v", err)
	}
	must(os.MkdirAll(absOut, 0o700))

	manifest := hvm_network.Manifest{
		SchemaVersion:             1,
		Mode:                      *mode,
		NodesCount:                *nodesN,
		ChainID:                   1337,
		ActivationHeight:          uint64(*nodesN + 2),
		LegacyPoWDifficulty:       1,
		PoHTicksPerBlock:          4_000,
		ConsensusActivationHeight: uint64(*nodesN + 4),
		FeeCollector:              "0x000000000000000000000000000000000000fee0",
		SlashCollector:            "0x00000000000000000000000000000000000051a5",
		Validator: hvm_network.ValidatorConfig{
			MinBondUnits:       10 * blockchain.AmountScale,
			ActivationDelay:    1,
			UnbondingBlocks:    8,
			JailBlocks:         100,
			DoubleVoteSlashBPS: 500,
		},
		ConsensusNetwork: hvm_network.NetworkConfig{
			ProposalTimeoutMS:  800,
			PrevoteTimeoutMS:   600,
			PrecommitTimeoutMS: 600,
			RoundDeltaMS:       100,
			MaxRound:           64,
			MaxMessageBytes:    4 << 20,
		},
	}

	fixtures := make([]validatorFixture, *nodesN)
	for i := 0; i < *nodesN; i++ {
		fixtures[i] = makeValidator(i, *mode)
		manifest.Nodes = append(manifest.Nodes, fixtures[i].pub)
	}

	recorder, err := wallet.NewWallet()
	must(err)
	manifest.RecorderAddress = recorder.Address()
	must(hvm_network.WriteJSON(filepath.Join(absOut, "secrets", "recorder.json"), hvm_network.RecorderSecret{PrivateHex: hex.EncodeToString(recorder.PrivateKeyBytes())}, 0o600))

	cfg := blockchain.DefaultProtocolV2Config()
	cfg.ChainID = manifest.ChainID
	cfg.ActivationHeight = manifest.ActivationHeight
	cfg.LegacyPoWDifficulty = manifest.LegacyPoWDifficulty
	cfg.PoHTicksPerBlock = manifest.PoHTicksPerBlock
	cfg.ConsensusActivationHeight = manifest.ConsensusActivationHeight
	cfg.Validator.MinBondUnits = manifest.Validator.MinBondUnits
	cfg.Validator.ActivationDelay = manifest.Validator.ActivationDelay
	cfg.Validator.UnbondingBlocks = manifest.Validator.UnbondingBlocks
	cfg.Validator.JailBlocks = manifest.Validator.JailBlocks
	cfg.Validator.DoubleVoteSlashBPS = manifest.Validator.DoubleVoteSlashBPS
	cfg.ConsensusNetwork.ProposalTimeout = time.Duration(manifest.ConsensusNetwork.ProposalTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.PrevoteTimeout = time.Duration(manifest.ConsensusNetwork.PrevoteTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.PrecommitTimeout = time.Duration(manifest.ConsensusNetwork.PrecommitTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.RoundTimeoutDelta = time.Duration(manifest.ConsensusNetwork.RoundDeltaMS) * time.Millisecond
	cfg.ConsensusNetwork.MaxRound = manifest.ConsensusNetwork.MaxRound
	cfg.ConsensusNetwork.MaxFutureHeight = 4
	cfg.ConsensusNetwork.MaxMessageBytes = manifest.ConsensusNetwork.MaxMessageBytes
	cfg.RequireNodeRegistrationV2 = true

	bootstrapDir := filepath.Join(absOut, "bootstrap")
	bc := blockchain.NewBlockchainWithDirAndV2Config(bootstrapDir, cfg)

	for i, v := range fixtures {
		rec := blockchain.NodeRecordV2{
			NodeRecord: blockchain.NodeRecord{
				NodeID:      v.pub.NodeID,
				PeerID:      v.pub.PeerID,
				Multiaddrs:  []string{v.pub.P2PMultiaddr},
				TEPPubkey:   v.pub.TEPPubkey,
				TEPPort:     47777,
				RPCEndpoint: fmt.Sprintf("http://%s:%d/rpc", v.pub.IP, v.pub.RPCPort),
				ExternalIP:  v.pub.IP,
				Version:     "hvm-network-devnet",
				ChainID:     int(manifest.ChainID),
			},
			NodeClass:       blockchain.NodeClass(v.pub.NodeClass),
			Roles:           stringRoles(v.pub.Roles),
			TEPEnabled:      true,
			Capabilities:    []string{blockchain.NodeCapabilityHVMV2, blockchain.NodeCapabilityValidatorV2, blockchain.NodeCapabilityConsensusV2},
			ProtocolVersion: blockchain.BlockVersionV2,
		}
		tx := blockchain.NewNodeRegistrationV2(v.operator.Address(), rec)
		must(tx.Sign(v.operator))
		bc.SetPendingTransactions([]*blockchain.Transaction{tx}, nil)
		if err := bc.AddBlock(v.operator.Address()); err != nil {
			fatalf("node registration %d: %v", i, err)
		}
	}

	// One pre-V2 funding block for the independent MPR recorder/signer.
	bc.SetPendingTransactions(nil, nil)
	must(bc.AddBlock(recorder.Address()))

	regs := make([]*protocolv2.TransactionV2, 0, len(fixtures))
	for _, v := range fixtures {
		req := consensus.RegisterRequest{
			NodeID: v.pub.NodeID, PeerID: v.pub.PeerID,
			ConsensusPubKey: v.consensus.PublicKeyHexCompressed(),
			RewardAddress:   v.reward.Address(), BondUnits: cfg.Validator.MinBondUnits,
		}
		data, _ := json.Marshal(req)
		fee, err := cfg.FeePolicy.ComputeFee(validatorRegisterCompute)
		must(err)
		tx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxValidatorRegister, v.operator.Address(), "", cfg.Validator.MinBondUnits, 0, validatorRegisterCompute, fee, data)
		must(tx.Sign(v.operator))
		regs = append(regs, tx)
	}
	bc.SetPendingTransactions(nil, regs)
	must(bc.AddBlock(fixtures[0].operator.Address()))

	// Advance PENDING validators to ACTIVE without enabling BFT yet.
	bc.SetPendingTransactions(nil, nil)
	must(bc.AddBlock(fixtures[0].operator.Address()))
	manifest.BootstrapHeight = bc.Height()
	if uint64(manifest.BootstrapHeight+1) != manifest.ConsensusActivationHeight {
		fatalf("bootstrap height %d inconsistent with consensus activation %d", manifest.BootstrapHeight, manifest.ConsensusActivationHeight)
	}

	set := bc.CurrentValidatorSet(uint64(manifest.BootstrapHeight + 1))
	if len(set.Validators) != *nodesN {
		fatalf("active validator set=%d want %d", len(set.Validators), *nodesN)
	}

	for i, v := range fixtures {
		nodeDir := filepath.Join(absOut, fmt.Sprintf("node%d", i))
		must(os.MkdirAll(filepath.Join(nodeDir, "data"), 0o700))
		must(copyDir(bootstrapDir, filepath.Join(nodeDir, "data")))
		must(hvm_network.WriteJSON(filepath.Join(nodeDir, "secrets.json"), v.secret, 0o600))
	}
	must(hvm_network.WriteJSON(filepath.Join(absOut, "manifest.json"), manifest, 0o644))
	fmt.Printf("HVM_NETWORK_FIXTURE_OK out=%s nodes=%d bootstrap_height=%d consensus_height=%d\n", absOut, *nodesN, manifest.BootstrapHeight, manifest.ConsensusActivationHeight)
}

func makeValidator(i int, mode string) validatorFixture {
	op, err := wallet.NewWallet()
	must(err)
	reward, err := wallet.NewWallet()
	must(err)
	cv, err := wallet.NewWallet()
	must(err)
	validatorID, err := consensus.ValidatorID(cv.PublicKeyHexCompressed())
	must(err)

	p2pPriv, _, err := libcrypto.GenerateEd25519Key(rand.Reader)
	must(err)
	peerID, err := peer.IDFromPrivateKey(p2pPriv)
	must(err)
	p2pRaw, err := libcrypto.MarshalPrivateKey(p2pPriv)
	must(err)

	ip := "127.0.0.1"
	p2pPort := 31301 + i
	rpcPort := 18101 + i
	ns := ""
	rootVeth := ""
	nsVeth := ""
	if mode == "netns" {
		ip = fmt.Sprintf("10.203.0.%d", 11+i)
		p2pPort = 30307
		rpcPort = 18009
		ns = fmt.Sprintf("hb3e-n%d", i)
		rootVeth = fmt.Sprintf("hb3e-r%d", i)
		nsVeth = "eth0"
	}
	class := string(blockchain.NodeClassNode)
	roles := []string{string(blockchain.NodeRoleFull)}
	storageRole := "secondary"
	if i == 0 {
		class = string(blockchain.NodeClassHPC)
		roles = []string{string(blockchain.NodeRoleFull), string(blockchain.NodeRoleBlockchain)}
		storageRole = "none"
	} else if i == 1 {
		roles = []string{string(blockchain.NodeRoleFull), string(blockchain.NodeRoleEdge)}
		storageRole = "edge"
	} else if i == 2 {
		roles = []string{string(blockchain.NodeRoleFull), string(blockchain.NodeRoleStorage)}
	}
	nodeID := fmt.Sprintf("hvm-network-%s-%d", class, i)
	tep := sha256.Sum256([]byte("hvm-network-tep:" + nodeID))
	pub := hvm_network.NodePublic{
		Index: i, NodeID: nodeID, NodeClass: class, Roles: roles,
		PeerID: peerID.String(), ValidatorID: validatorID,
		OperatorAddress: op.Address(), RewardAddress: reward.Address(), ConsensusAddress: cv.Address(),
		IP: ip, P2PPort: p2pPort, RPCPort: rpcPort,
		P2PMultiaddr: fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip, p2pPort, peerID.String()),
		Namespace:    ns, RootVeth: rootVeth, NamespaceVeth: nsVeth,
		TEPPubkey: hex.EncodeToString(tep[:]), StorageRole: storageRole,
		ConsensusProtocol: "/hashburst/consensus/2.0.0",
	}
	secret := hvm_network.NodeSecret{
		OperatorPrivateHex:  hex.EncodeToString(op.PrivateKeyBytes()),
		RewardPrivateHex:    hex.EncodeToString(reward.PrivateKeyBytes()),
		ConsensusPrivateHex: hex.EncodeToString(cv.PrivateKeyBytes()),
		P2PPrivateBase64:    base64.StdEncoding.EncodeToString(p2pRaw),
	}
	return validatorFixture{pub: pub, secret: secret, operator: op, reward: reward, consensus: cv}
}

func stringRoles(in []string) []blockchain.NodeRole {
	out := make([]blockchain.NodeRole, 0, len(in))
	for _, s := range in {
		out = append(out, blockchain.NodeRole(strings.TrimSpace(s)))
	}
	return out
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "hvm-network-fixture: "+format+"\n", args...)
	os.Exit(1)
}
