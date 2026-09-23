package testnet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hashburst/blockchain"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Opt-in integration: builds real fixture/node binaries and runs four OS processes.
func TestFourPersistentValidatorProcesses(t *testing.T) {
	if os.Getenv("HB_TESTNET_INTEGRATION") != "1" {
		t.Skip("set HB_TESTNET_INTEGRATION=1 for process integration")
	}
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	tmp := t.TempDir()
	goexe := filepath.Join(runtime.GOROOT(), "bin/go")
	build := func(pkg, name string) string {
		p := filepath.Join(tmp, name)
		cmd := exec.Command(goexe, "build", "-buildvcs=false", "-o", p, pkg)
		cmd.Dir = root
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("build: %v %s", e, b)
		}
		return p
	}
	fixtureBin := build("./devnet/phase3e/cmd/fixture", "fixture")
	nodeBin := build("./cmd/hashburst-testnet", "node")
	out := filepath.Join(tmp, "devnet/phase3e/run-persistent")
	if b, e := exec.Command(fixtureBin, "--out", out, "--nodes", "4", "--mode", "loopback").CombinedOutput(); e != nil {
		t.Fatalf("fixture: %v %s", e, b)
	}
	var m struct {
		ChainID    uint64 `json:"chain_id"`
		Activation uint64 `json:"activation_height"`
		Consensus  uint64 `json:"consensus_activation_height"`
		Height     int    `json:"bootstrap_height"`
		Nodes      []struct {
			NodeID      string `json:"node_id"`
			PeerID      string `json:"peer_id"`
			ValidatorID string `json:"validator_id"`
		} `json:"nodes"`
	}
	raw, _ := os.ReadFile(filepath.Join(out, "manifest.json"))
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	cfg := blockchain.DefaultProtocolV2Config()
	cfg.ChainID = m.ChainID
	cfg.ActivationHeight = m.Activation
	cfg.ConsensusActivationHeight = m.Consensus
	cfg.LegacyPoWDifficulty = 1
	cfg.PoHTicksPerBlock = 4000
	cfg.Validator.MinBondUnits = 10 * blockchain.AmountScale
	cfg.Validator.ActivationDelay = 1
	cfg.Validator.UnbondingBlocks = 8
	cfg.Validator.JailBlocks = 100
	cfg.ConsensusNetwork.ProposalTimeout = 800 * time.Millisecond
	cfg.ConsensusNetwork.PrevoteTimeout = 600 * time.Millisecond
	cfg.ConsensusNetwork.PrecommitTimeout = 600 * time.Millisecond
	cfg.ConsensusNetwork.RoundTimeoutDelta = 100 * time.Millisecond
	cfg.ConsensusNetwork.MaxFutureHeight = 4
	blocks, e := blockchain.NewChainStorage(filepath.Join(out, "bootstrap")).LoadAll()
	if e != nil {
		t.Fatal(e)
	}
	configs := make([]Config, 4)
	paths := make([]string, 4)
	commands := make([]*exec.Cmd, 4)
	logs := make([]*bytes.Buffer, 4)
	// Select unused loopback ports for this isolated integration run.
	for i, n := range m.Nodes {
		d := filepath.Join(out, fmt.Sprintf("node%d", i))
		var secret struct {
			P2P       string `json:"p2p_private_base64"`
			Consensus string `json:"consensus_private_hex"`
		}
		b, _ := os.ReadFile(filepath.Join(d, "secrets.json"))
		if e = json.Unmarshal(b, &secret); e != nil {
			t.Fatal(e)
		}
		os.WriteFile(filepath.Join(d, "p2p.key"), []byte(secret.P2P), 0600)
		os.WriteFile(filepath.Join(d, "consensus.key"), []byte(secret.Consensus), 0600)
		configs[i] = Config{Schema: 1, Network: "testnet", NodeID: n.NodeID, Role: "validator", ValidatorID: n.ValidatorID, PeerID: n.PeerID, DataDir: filepath.Join(d, "data"), P2PKeyFile: filepath.Join(d, "p2p.key"), ConsensusKeyFile: filepath.Join(d, "consensus.key"), RPCListen: fmt.Sprintf("127.0.0.1:%d", freePort(t)), P2PListenIP: "127.0.0.1", P2PPort: freePort(t), GenesisHash: blocks[0].Hash, CheckpointHeight: m.Height, CheckpointHash: blocks[m.Height].Hash, Protocol: cfg}
	}
	for i := range configs {
		for j, c := range configs {
			if i != j {
				configs[i].Bootnodes = append(configs[i].Bootnodes, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", c.P2PPort, c.PeerID))
			}
		}
		paths[i] = filepath.Join(out, fmt.Sprintf("config%d.json", i))
		b, _ := json.Marshal(configs[i])
		os.WriteFile(paths[i], b, 0600)
		if b, e := exec.Command(nodeBin, "--config", paths[i], "--provision").CombinedOutput(); e != nil {
			t.Fatalf("provision %d: %v %s", i, e, b)
		}
	}
	stop := func(i int) {
		if commands[i] != nil {
			_ = commands[i].Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- commands[i].Wait() }()
			select {
			case e := <-done:
				if e != nil {
					t.Errorf("exit %d: %v", i, e)
				}
			case <-time.After(20 * time.Second):
				_ = commands[i].Process.Kill()
				<-done
				t.Errorf("shutdown timeout %d", i)
			}
			commands[i] = nil
		}
	}
	defer func() {
		for i := range commands {
			stop(i)
		}
		if t.Failed() {
			for i, b := range logs {
				if b != nil {
					t.Logf("node%d: %s", i, b.String())
				}
			}
		}
	}()
	start := func(i int) {
		cmd := exec.Command(nodeBin, "--config", paths[i])
		logs[i] = new(bytes.Buffer)
		cmd.Stdout = logs[i]
		cmd.Stderr = logs[i]
		if e := cmd.Start(); e != nil {
			t.Fatal(e)
		}
		commands[i] = cmd
	}
	for i := range commands {
		start(i)
	}
	client := &http.Client{Timeout: time.Second}
	wait := func(target int) {
		deadline := time.Now().Add(50 * time.Second)
		for time.Now().Before(deadline) {
			ok := true
			for _, c := range configs {
				resp, e := client.Get("http://" + c.RPCListen + "/health")
				if e != nil {
					ok = false
					continue
				}
				var h struct {
					Finalized int `json:"finalized_height"`
				}
				e = json.NewDecoder(resp.Body).Decode(&h)
				resp.Body.Close()
				if e != nil || h.Finalized < target {
					ok = false
				}
			}
			if ok {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("no all-node finality >= %d", target)
	}
	wait(m.Height + 4)
	stop(3)
	resp, err := client.Get("http://" + configs[0].RPCListen + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Finalized int `json:"finalized_height"`
	}
	err = json.NewDecoder(resp.Body).Decode(&baseline)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Restart the same persistent identity as observer; preserve signing journals.
	journal := filepath.Join(configs[3].DataDir, "consensus-bft-signatures.jsonl")
	before, _ := os.ReadFile(journal)
	configs[3].Role = "observer"
	configs[3].ConsensusKeyFile = ""
	b, _ := json.Marshal(configs[3])
	os.WriteFile(paths[3], b, 0600)
	start(3)
	wait(baseline.Finalized + 4)
	stop(3)
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(before, after) {
		t.Fatal("observer modified signing journal")
	}
	t.Log("PERSISTENT_TESTNET_FOUR_PROCESS_FINALITY_AND_OBSERVER_RESTART_OK")
}
