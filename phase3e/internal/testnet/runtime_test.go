package testnet

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"hashburst/blockchain"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) Config {
	t.Helper()
	d := t.TempDir()
	cfg := blockchain.DefaultProtocolV2Config()
	bc := blockchain.NewBlockchainWithDirAndV2Config(d, cfg)
	key, _, e := crypto.GenerateEd25519Key(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := crypto.MarshalPrivateKey(key)
	id, _ := peer.IDFromPrivateKey(key)
	kp := filepath.Join(d, "p2p.key")
	if e = os.WriteFile(kp, []byte(base64.StdEncoding.EncodeToString(raw)), 0600); e != nil {
		t.Fatal(e)
	}
	return Config{Schema: 1, Network: "testnet", NodeID: "test", Role: "observer", DataDir: d, RPCListen: "127.0.0.1:" + strconv.Itoa(freePort(t)), P2PListenIP: "127.0.0.1", P2PPort: freePort(t), PeerID: id.String(), P2PKeyFile: kp, GenesisHash: bc.HeadSnapshot().Hash, CheckpointHash: bc.HeadSnapshot().Hash, Bootnodes: []string{"/ip4/127.0.0.1/tcp/65534/p2p/" + id.String()}, Protocol: cfg}
}
func freePort(t *testing.T) int {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func TestStatePinLockAndRestart(t *testing.T) {
	c := fixture(t)
	s, e := Prepare(c, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Prepare(c, false); e == nil {
		t.Fatal("duplicate process accepted")
	}
	s.Close()
	s, e = Prepare(c, false)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	c.Protocol.ChainID++
	if _, e = Prepare(c, false); e == nil {
		t.Fatal("changed chain ID accepted")
	}
	c.Protocol.ChainID--
	c.Protocol.ConsensusNetwork.MaxRound++
	if _, e = Prepare(c, false); e == nil {
		t.Fatal("changed protocol accepted")
	}
}
func TestRejectMissingCorruptStateAndJournals(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "journal", "genesis", "index"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t)
			s, e := Prepare(c, true)
			if e != nil {
				t.Fatal(e)
			}
			s.Close()
			switch kind {
			case "missing":
				os.Remove(filepath.Join(c.DataDir, "blockchain.dat"))
			case "corrupt":
				os.WriteFile(filepath.Join(c.DataDir, "blockchain.dat"), []byte{0, 0}, 0600)
			case "journal":
				os.WriteFile(filepath.Join(c.DataDir, "consensus-bft-signatures.jsonl"), []byte("invalid\n"), 0600)
			case "index":
				os.WriteFile(filepath.Join(c.DataDir, "blockchain.idx"), []byte("bad"), 0600)
			case "genesis":
				c.GenesisHash = strings.Repeat("a", 64)
			}
			before, _ := os.ReadFile(filepath.Join(c.DataDir, "blockchain.dat"))
			if _, e = Prepare(c, false); e == nil {
				t.Fatal("bad state accepted")
			}
			after, _ := os.ReadFile(filepath.Join(c.DataDir, "blockchain.dat"))
			if string(before) != string(after) {
				t.Fatal("state mutated on error")
			}
		})
	}
}
func TestConfigurationRejectsPublicRPCAndUnknownFields(t *testing.T) {
	c := fixture(t)
	c.RPCListen = "0.0.0.0:8099"
	if c.Validate() == nil {
		t.Fatal("public RPC accepted")
	}
	c = fixture(t)
	b, _ := json.Marshal(c)
	b = append(b[:len(b)-1], []byte(",\"typo\":true}")...)
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, b, 0600)
	if _, e := Load(p); e == nil {
		t.Fatal("unknown key accepted")
	}
	c = fixture(t)
	c.DataDir = "/var/lib/hashburst"
	if c.Validate() == nil {
		t.Fatal("legacy directory accepted")
	}
}
func TestPersistentObserverLifecycle(t *testing.T) {
	c := fixture(t)
	s, e := Prepare(c, true)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	client := &http.Client{Timeout: time.Second}
	for cycle := 0; cycle < 2; cycle++ {
		s, e = Prepare(c, false)
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- Run(ctx, s) }()
		ready := false
		for i := 0; i < 60; i++ {
			r, e := client.Get("http://" + c.RPCListen + "/health")
			if e == nil {
				r.Body.Close()
				ready = r.StatusCode == 200
				if ready {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		if ready {
			r, e := client.Post("http://"+c.RPCListen+"/control/start", "application/json", strings.NewReader("{}"))
			if e != nil {
				t.Error(e)
			} else {
				r.Body.Close()
				if r.StatusCode != 404 {
					t.Error("admin route exposed")
				}
			}
		}
		cancel()
		select {
		case e = <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("shutdown stuck")
		}
		s.Close()
		if !ready {
			t.Fatal("HTTP never ready")
		}
	}
}
