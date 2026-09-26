package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"hashburst/internal/testnet"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBootstrapFourProcessesRestart(t *testing.T) {
	if os.Getenv("HB_BOOTSTRAP_INTEGRATION") != "1" {
		t.Skip("set HB_BOOTSTRAP_INTEGRATION=1")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "node")
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-buildvcs=false", "-o", bin, "../../cmd/hashburst-testnet")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build %v %s", e, b)
	}
	ids := []Identity{}
	for i := byte(0); i < 4; i++ {
		ids = append(ids, identity(t, root, i))
	}
	out := filepath.Join(root, "network")
	if e := Assemble(out, 987654321, ids); e != nil {
		t.Fatal(e)
	}
	free := func() int {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		return p
	}
	configs := make([]testnet.Config, 4)
	paths := make([]string, 4)
	for i := range configs {
		name := fmt.Sprintf("node%c", 'a'+i)
		c, e := testnet.Load(filepath.Join(out, name+".validator.json"))
		if e != nil {
			t.Fatal(e)
		}
		c.DataDir = filepath.Join(root, name, "state")
		c.P2PKeyFile = filepath.Join(root, name, "p2p.key")
		c.ConsensusKeyFile = filepath.Join(root, name, "consensus.key")
		c.P2PListenIP = "127.0.0.1"
		c.P2PPort = free()
		c.RPCListen = fmt.Sprintf("127.0.0.1:%d", free())
		c.Bootnodes = nil
		if e = os.Mkdir(c.DataDir, 0700); e != nil {
			t.Fatal(e)
		}
		for _, name := range []string{"blockchain.dat", "blockchain.idx"} {
			b, e := os.ReadFile(filepath.Join(out, "checkpoint", name))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(c.DataDir, name), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
		configs[i] = c
	}
	for i := range configs {
		for j, c := range configs {
			if i != j {
				configs[i].Bootnodes = append(configs[i].Bootnodes, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", c.P2PPort, c.PeerID))
			}
		}
		s, e := testnet.Prepare(configs[i], true)
		if e != nil {
			t.Fatal(e)
		}
		s.Close()
		paths[i] = filepath.Join(root, fmt.Sprintf("config%d.json", i))
		if e = jsonFile(paths[i], configs[i]); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	processes := make([]*exec.Cmd, 4)
	logs := make([]*os.File, 4)
	start := func(i int) {
		cmd := exec.CommandContext(ctx, bin, "--config", paths[i])
		f, e := os.OpenFile(filepath.Join(root, fmt.Sprintf("node%d.log", i)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		logs[i] = f
		cmd.Stdout = f
		cmd.Stderr = f
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		processes[i] = cmd
	}
	stop := func(i int) {
		if processes[i] != nil {
			processes[i].Process.Kill()
			processes[i].Wait()
			logs[i].Close()
			processes[i] = nil
		}
	}
	defer func() {
		for i := range processes {
			stop(i)
			if t.Failed() {
				b, _ := os.ReadFile(filepath.Join(root, fmt.Sprintf("node%d.log", i)))
				t.Log(string(b))
			}
		}
	}()
	for i := range processes {
		start(i)
	}
	client := &http.Client{Timeout: time.Second}
	wait := func(target int) {
		deadline := time.Now().Add(40 * time.Second)
		for time.Now().Before(deadline) {
			ok := true
			for _, c := range configs {
				r, e := client.Get("http://" + c.RPCListen + "/health")
				if e != nil {
					ok = false
					continue
				}
				var h struct {
					Chain  uint64 `json:"chain_id"`
					Height int    `json:"finalized_height"`
					Peers  int    `json:"peer_count"`
				}
				e = json.NewDecoder(r.Body).Decode(&h)
				r.Body.Close()
				if e != nil || h.Chain != 987654321 || h.Height < target || h.Peers < 3 {
					ok = false
				}
			}
			if ok {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("no shared finality >= %d", target)
	}
	wait(10)
	stop(0)
	start(0)
	wait(14)
	t.Log("BOOTSTRAP_FOUR_PROCESS_FINALITY_AND_RESTART_OK chain=987654321 (test-only ID)")
}
