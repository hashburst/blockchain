package blockchain

import (
	"bytes"
	"context"
	"encoding/json"
	"hashburst/wallet"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func apowTestConfig() ProtocolV2Config {
	c := phase3CTestConfig()
	c.ChainID = 4735490
	c.EVM = &EVMConfig{ActivationHeight: 7, GasLimit: 1000000, BaseFeeWei: 1}
	c.APoW = &APoWConfig{ActivationHeight: 7, InitialBits: 4, MinBits: 1, MaxBits: 12, Window: 4, TargetSeconds: 5}
	return c
}
func TestAPoWFourValidatorsRewardReplay(t *testing.T) {
	s := setupPhase3DChainsWithConfig(t, apowTestConfig())
	miner, _ := wallet.NewWallet()
	beneficiary, _ := wallet.NewWallet()
	for round := 0; round < 5; round++ {
		job, err := s.nodes[0].APoWJob()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		proof, err := MineAPoW(ctx, *job, miner, beneficiary.Address())
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range s.nodes {
			if err = n.SubmitAPoW(*proof); err != nil {
				t.Fatal(err)
			}
		}
		b := finalizeEVMFixture(t, s)
		wire := blockToWire(b)
		encoded, _ := json.Marshal(wire)
		var decoded blockWire
		if err = json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.toBlock().GenerateHash() != b.Hash || decoded.APoW == nil {
			t.Fatal("APoW chain sync encoding lost proof")
		}
		if b.APoW == nil || b.Version != BlockVersionAPoW || !strings.EqualFold(b.APoW.Author, miner.Address()) {
			t.Fatal("work author lost")
		}
		if err = validateRewardRecipient(b, beneficiary.Address()); err != nil {
			t.Fatal(err)
		}
		if b.Transactions[0].Amount != 50 {
			t.Fatal("subsidy not 50 HBT")
		}
		if err = s.nodes[0].SubmitAPoW(*proof); err == nil {
			t.Fatal("reused proof accepted")
		}
		for _, n := range s.nodes {
			if err = n.VerifyChain(); err != nil {
				t.Fatal(err)
			}
			if err = n.rebuildProjections(n.Blocks); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAPoWPersistentOpenAndLocalIngress(t *testing.T) {
	s := setupPhase3DChainsWithConfig(t, apowTestConfig())
	bc := s.nodes[0]
	w, _ := wallet.NewWallet()
	job, _ := bc.APoWJob()
	proof, err := MineAPoW(context.Background(), *job, w, w.Address())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(proof)
	handler := bc.APoWHandler(nil)
	req := httptest.NewRequest("POST", "/apow", bytes.NewReader(raw))
	req.RemoteAddr = "203.0.113.1:1234"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 403 {
		t.Fatal("remote mining ingress exposed")
	}
	req = httptest.NewRequest("POST", "/apow", bytes.NewReader(raw))
	req.RemoteAddr = "127.0.0.1:1234"
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	finalizeEVMFixture(t, s)
	journal := filepath.Join(bc.storage.dir, "consensus-bft-signatures.jsonl")
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenExistingBlockchain(bc.storage.dir, s.cfg, bc.Blocks[0].Hash, 6, bc.Blocks[6].Hash)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(before, after) || opened.HeadSnapshot().Hash != bc.HeadSnapshot().Hash {
		t.Fatal("persistent state changed on open")
	}
}

func TestAPoWProofTampering(t *testing.T) {
	s := setupPhase3DChainsWithConfig(t, apowTestConfig())
	w, _ := wallet.NewWallet()
	other, _ := wallet.NewWallet()
	job, _ := s.nodes[0].APoWJob()
	p, err := MineAPoW(context.Background(), *job, w, w.Address())
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*APoWProof){func(p *APoWProof) { p.ChainID++ }, func(p *APoWProof) { p.Height++ }, func(p *APoWProof) { p.ParentHash = strings.Repeat("0", 64) }, func(p *APoWProof) { p.PoH++ }, func(p *APoWProof) { p.Beneficiary = strings.ToLower(other.Address()) }, func(p *APoWProof) { p.Bits++ }, func(p *APoWProof) { p.EpochStart++ }, func(p *APoWProof) { p.Signature = strings.Repeat("0", 130) }}
	for i, change := range changes {
		copy := *p
		change(&copy)
		if err = s.nodes[0].SubmitAPoW(copy); err == nil {
			t.Fatalf("accepted tampering %d", i)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job.Bits = 240
	if _, err = MineAPoW(ctx, *job, w, w.Address()); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestAPoWActivationAndRetarget(t *testing.T) {
	cfg := apowTestConfig()
	prev := &Block{Index: 6, Timestamp: time.Unix(100, 0)}
	bits, start, err := expectedAPoW(prev, cfg)
	if err != nil || bits != 4 || start != 100 {
		t.Fatal(bits, start, err)
	}
	prev.Index = 10
	prev.APoW = &APoWProof{Bits: 4, EpochStart: 100}
	prev.Timestamp = time.Unix(101, 0)
	bits, start, err = expectedAPoW(prev, cfg)
	if err != nil || bits != 5 || start != 101 {
		t.Fatal(bits, start, err)
	}
	prev.Timestamp = time.Unix(200, 0)
	bits, _, _ = expectedAPoW(prev, cfg)
	if bits != 3 {
		t.Fatal(bits)
	}
	prev.APoW.Bits = cfg.APoW.MinBits
	bits, _, _ = expectedAPoW(prev, cfg)
	if bits != cfg.APoW.MinBits {
		t.Fatal("lower bound")
	}
	cfg.APoW = nil
	raw, _ := json.Marshal(cfg)
	if strings.Contains(string(raw), "apow") {
		t.Fatal("legacy config digest changed")
	}
	if err := validateAPoWEnvelope(prev, &Block{Index: 11, APoW: &APoWProof{}}, cfg); err == nil {
		t.Fatal("preactivation proof accepted")
	}
}
func TestAPoWReactorStartsWithoutWork(t *testing.T) {
	s := setupPhase3DChainsWithConfig(t, apowTestConfig())
	set := s.nodes[0].CurrentValidatorSet(7)
	p, _ := set.Proposer(7, 0)
	v := s.byID[strings.ToLower(p.ID)]
	r, err := NewConsensusReactor(s.nodes[0], p.ID, v.consensus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if !r.Running() {
		t.Fatal("reactor not running")
	}
	if _, err = s.nodes[0].BuildConsensusProposal(p.ID, 0); err != ErrAPoWUnavailable {
		t.Fatal(err)
	}
}
