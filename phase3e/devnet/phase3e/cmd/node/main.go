package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"hashburst/blockchain"
	phase3e "hashburst/devnet/phase3e/internal/devnet"
	"hashburst/wallet"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

type consensusRunner interface {
	Start() error
	Run(context.Context) error
	Running() bool
	TryStatus() (blockchain.ConsensusReactorStatus, bool)
}

type runtime struct {
	controlMu  sync.Mutex // Serialize control requests through full Run shutdown.
	consDone   chan struct{}
	mu         sync.Mutex
	manifest   phase3e.Manifest
	self       phase3e.NodePublic
	bc         *blockchain.Blockchain
	mp         *blockchain.Mempool
	p2p        *blockchain.P2PNode
	syncer     *blockchain.Syncer
	reactor    consensusRunner
	consNet    *blockchain.Libp2pConsensusNetwork
	consCtx    context.Context
	consCancel context.CancelFunc
	started    bool
}

func main() {
	manifestPath := flag.String("manifest", "", "public manifest path")
	nodeDir := flag.String("node-dir", "", "node runtime directory")
	index := flag.Int("index", -1, "node index")
	flag.Parse()
	if *manifestPath == "" || *nodeDir == "" || *index < 0 {
		log.Fatal("--manifest, --node-dir and --index are required")
	}

	var manifest phase3e.Manifest
	must(phase3e.LoadJSON(*manifestPath, &manifest))
	if *index >= len(manifest.Nodes) {
		log.Fatalf("node index %d outside manifest", *index)
	}
	self := manifest.Nodes[*index]
	var secret phase3e.NodeSecret
	must(phase3e.LoadJSON(filepath.Join(*nodeDir, "secrets.json"), &secret))

	consensusWallet, err := wallet.FromPrivateKeyHex(secret.ConsensusPrivateHex)
	must(err)
	if !wallet.AddressEqual(consensusWallet.Address(), self.ConsensusAddress) {
		log.Fatal("consensus secret does not match manifest")
	}
	p2pRaw, err := base64.StdEncoding.DecodeString(secret.P2PPrivateBase64)
	must(err)
	p2pPriv, err := libcrypto.UnmarshalPrivateKey(p2pRaw)
	must(err)

	cfg := protocolConfig(manifest)
	dataDir := filepath.Join(*nodeDir, "data")
	bc := blockchain.NewBlockchainWithDirAndV2Config(dataDir, cfg)
	if bc.Height() < manifest.BootstrapHeight {
		log.Fatalf("node chain height=%d below bootstrap=%d", bc.Height(), manifest.BootstrapHeight)
	}
	mp := blockchain.NewMempool()
	bc.SetMempool(mp)
	p2pListenIP := "127.0.0.1"
	if manifest.Mode == "netns" {
		p2pListenIP = "0.0.0.0"
	}
	p2pNode, err := blockchain.NewP2PNodeWithListenIP(bc, mp, p2pListenIP, self.P2PPort, p2pPriv)
	must(err)
	syncer := blockchain.NewSyncer(bc, mp, p2pNode.Host)
	bc.SetSyncer(syncer)

	reactor, err := blockchain.NewConsensusReactor(bc, self.ValidatorID, consensusWallet, nil)
	must(err)
	consNet, err := blockchain.NewLibp2pConsensusNetwork(p2pNode.Host, reactor, cfg.ConsensusNetwork)
	must(err)

	rt := &runtime{manifest: manifest, self: self, bc: bc, mp: mp, p2p: p2pNode, syncer: syncer, reactor: reactor, consNet: consNet}

	rpc := blockchain.NewRPCHandler(bc, mp, int64(manifest.ChainID))
	rpc.SetV2Broadcaster(syncer)
	mux := http.NewServeMux()
	mux.Handle("/rpc", rpc)
	mux.HandleFunc("/health", rt.health)
	mux.HandleFunc("/control/start", rt.startConsensus)
	mux.HandleFunc("/control/stop", rt.stopConsensus)
	bindIP := "127.0.0.1"
	if manifest.Mode == "netns" {
		bindIP = "0.0.0.0"
	}
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", bindIP, self.RPCPort), Handler: mux, ReadHeaderTimeout: 3 * time.Second}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go rt.connectLoop(ctx)
	go rt.helloLoop(ctx)
	go func() {
		log.Printf("phase3e node %d HTTP/RPC on :%d peer=%s validator=%s", self.Index, self.RPCPort, self.PeerID, self.ValidatorID)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
			cancel()
		}
	}()

	<-ctx.Done()
	rt.stopConsensusInternal()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = server.Shutdown(shutdownCtx)
	shutdownCancel()
	_ = p2pNode.Host.Close()
}

func protocolConfig(m phase3e.Manifest) blockchain.ProtocolV2Config {
	cfg := blockchain.DefaultProtocolV2Config()
	cfg.ChainID = m.ChainID
	cfg.ActivationHeight = m.ActivationHeight
	cfg.LegacyPoWDifficulty = m.LegacyPoWDifficulty
	cfg.PoHTicksPerBlock = m.PoHTicksPerBlock
	cfg.ConsensusActivationHeight = m.ConsensusActivationHeight
	cfg.Validator.MinBondUnits = m.Validator.MinBondUnits
	cfg.Validator.ActivationDelay = m.Validator.ActivationDelay
	cfg.Validator.UnbondingBlocks = m.Validator.UnbondingBlocks
	cfg.Validator.JailBlocks = m.Validator.JailBlocks
	cfg.Validator.DoubleVoteSlashBPS = m.Validator.DoubleVoteSlashBPS
	cfg.ConsensusNetwork.ProposalTimeout = time.Duration(m.ConsensusNetwork.ProposalTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.PrevoteTimeout = time.Duration(m.ConsensusNetwork.PrevoteTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.PrecommitTimeout = time.Duration(m.ConsensusNetwork.PrecommitTimeoutMS) * time.Millisecond
	cfg.ConsensusNetwork.RoundTimeoutDelta = time.Duration(m.ConsensusNetwork.RoundDeltaMS) * time.Millisecond
	cfg.ConsensusNetwork.MaxRound = m.ConsensusNetwork.MaxRound
	cfg.ConsensusNetwork.MaxFutureHeight = 4
	cfg.ConsensusNetwork.MaxMessageBytes = m.ConsensusNetwork.MaxMessageBytes
	cfg.RequireNodeRegistrationV2 = true
	return cfg
}

func (rt *runtime) connectLoop(ctx context.Context) {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, n := range rt.manifest.Nodes {
			if n.Index == rt.self.Index {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			_ = rt.p2p.Connect(cctx, n.P2PMultiaddr)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (rt *runtime) helloLoop(ctx context.Context) {
	ticker := time.NewTicker(1200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.syncer.HelloAll()
		}
	}
}

func (rt *runtime) startConsensus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rt.controlMu.Lock()
	defer rt.controlMu.Unlock()
	rt.mu.Lock()
	if rt.started {
		rt.mu.Unlock()
		writeJSON(w, map[string]interface{}{"started": true, "already_running": true})
		return
	}
	// Start synchronously before acknowledging the control request. The old
	// path set rt.started first and launched reactor.Run in a goroutine, which
	// allowed the round-0 proposer to broadcast while peers reported "started"
	// even though their ConsensusReactor was not running yet.
	if err := rt.reactor.Start(); err != nil {
		log.Printf("control/start failed node=%d: %v", rt.self.Index, err)
		rt.mu.Unlock()
		http.Error(w, fmt.Sprintf("consensus start: %v", err), http.StatusConflict)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	rt.consDone = done
	rt.consCtx, rt.consCancel, rt.started = ctx, cancel, true
	rt.mu.Unlock()
	go func() {
		if err := rt.reactor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("consensus reactor stopped: %v", err)
		}
		rt.mu.Lock()
		rt.started = false
		rt.consCancel = nil
		close(done)
		rt.mu.Unlock()
	}()
	writeJSON(w, map[string]interface{}{"started": true})
}

func (rt *runtime) stopConsensus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	rt.stopConsensusInternal()
	writeJSON(w, map[string]interface{}{"stopped": true})
}

func (rt *runtime) stopConsensusInternal() {
	rt.controlMu.Lock()
	defer rt.controlMu.Unlock()
	rt.mu.Lock()
	cancel := rt.consCancel
	done := rt.consDone
	rt.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (rt *runtime) health(w http.ResponseWriter, _ *http.Request) {
	rt.mu.Lock()
	started := rt.started
	rt.mu.Unlock()
	reactor, reactorFresh := rt.reactor.TryStatus()
	reactorRunning := rt.reactor.Running()
	network := rt.consNet.Status()
	attached := rt.reactor != nil && rt.consNet != nil
	head := rt.bc.HeadSnapshot()
	headHash := ""
	validatorSetRoot := ""
	validatorStateRoot := ""
	receiptsRoot := ""
	if head != nil {
		headHash = head.Hash
		validatorSetRoot = head.ValidatorSetRoot
		validatorStateRoot = head.ValidatorStateRoot
		receiptsRoot = head.ReceiptsRoot
	}
	writeJSON(w, map[string]interface{}{
		"ok": true, "node_index": rt.self.Index, "node_id": rt.self.NodeID,
		"node_class": rt.self.NodeClass, "roles": rt.self.Roles,
		"height": rt.bc.Height(), "finalized_height": rt.bc.FinalizedHeight(), "head_hash": headHash,
		"bootstrap_height":  rt.manifest.BootstrapHeight,
		"peer_count":        len(rt.p2p.Host.Network().Peers()),
		"expected_peers":    rt.manifest.NodesCount - 1,
		"consensus_started": reactorRunning, "consensus_requested": started, "consensus_attached": attached,
		"reactor_status_fresh": reactorFresh, "reactor": reactor, "network": network,
		"hbt_state_root": rt.bc.HBTStateRoot(), "hvm_state_root": rt.bc.HVMStateRoot(),
		"receipts_root": receiptsRoot, "validator_set_root": validatorSetRoot,
		"validator_state_root": validatorStateRoot,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
