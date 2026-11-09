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
	opened, err := OpenExistingBlockchain(bc.storage.dir, s.cfg, testHistoryBlock(t,bc,0).Hash, 6, testHistoryBlock(t,bc,6).Hash)
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


func TestMainnetR6Height9RoundAbove64RequiresAPoWAndFinalizesWithWork(t *testing.T) {
	cfg := phase3CTestConfig()
	cfg.ChainID = MainnetChainID
	cfg.ConsensusActivationHeight = 9
	cfg.EVM = &EVMConfig{ActivationHeight: 9, GasLimit: 15000000, BaseFeeWei: 1000000000}
	// Keep production activation semantics while lowering only proof difficulty so
	// this liveness regression is deterministic and fast in CI.
	cfg.APoW = &APoWConfig{GasLimit: 15000000, ActivationHeight: 9, InitialBits: 4, MinBits: 1, MaxBits: 32, Window: 60, TargetSeconds: 60}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	s := setupPhase3DChainsWithConfig(t, cfg)
	// Reproduce the accepted R6 consensus boundary: four active validators and
	// a common finalized/pre-consensus head at height 8, with consensus+EVM+APoW
	// beginning exactly at height 9.
	for h := 7; h <= 8; h++ {
		s.nodes[0].SetPendingTransactions(nil, nil)
		if err := s.nodes[0].AddBlock(s.vals[0].operator.Address()); err != nil {
			t.Fatalf("build R6 boundary height %d: %v", h, err)
		}
		b := s.nodes[0].HeadSnapshot()
		for i := 1; i < 4; i++ {
			if err := s.nodes[i].AppendBlock(cloneBlockForConsensus(b)); err != nil {
				t.Fatalf("replicate R6 boundary height %d to validator %d: %v", h, i, err)
			}
		}
	}
	for i, bc := range s.nodes {
		if bc.Height() != 8 || len(bc.CurrentValidatorSet(9).Validators) != 4 {
			t.Fatalf("validator %d not at R6 height-8/four-validator boundary", i)
		}
	}
	bus, reactors := attachPhase3DReactors(t, s, []bool{true, true, true, true})
	for i, r := range reactors {
		// Model run #49 recovery after the historical round-64 lifetime boundary.
		r.height = 9
		r.round = 64
		r.reservedRound = 64
		r.lockedRound, r.validRound = -1, -1
		if err := r.persistRecoveryLocked(64); err != nil {
			t.Fatal(err)
		}
		if err := r.Start(); err != nil {
			t.Fatalf("validator %d resume above round 64: %v", i, err)
		}
		defer r.Stop()
		if r.round <= 64 {
			t.Fatalf("validator %d reused round %d", i, r.round)
		}
	}
	// With the production APoW activation boundary and no admitted work, even
	// the scheduled proposer cannot construct height 9. This is the exact
	// nil-prevote/round-change liveness signature observed in run #49/#50.
	set := s.nodes[0].CurrentValidatorSet(9)
	p, ok := set.Proposer(9, reactors[0].round)
	if !ok {
		t.Fatal("missing scheduled proposer")
	}
	var proposerNode *Blockchain
	for i, v := range s.vals {
		if strings.EqualFold(v.id, p.ID) {
			proposerNode = s.nodes[i]
			break
		}
	}
	if proposerNode == nil {
		t.Fatal("scheduled proposer fixture missing")
	}
	if _, err := proposerNode.BuildConsensusProposal(p.ID, reactors[0].round); err != ErrAPoWUnavailable {
		t.Fatalf("height 9 without work = %v, want ErrAPoWUnavailable", err)
	}
	for _, r := range reactors {
		if err := r.HandleTimeout(); err != nil {
			t.Fatal(err)
		}
	}
	bus.drain(20000)
	if s.nodes[0].FinalizedHeight() >= 9 {
		t.Fatal("height 9 finalized without APoW work")
	}

	miner, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.nodes[0].APoWJob()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	proof, err := MineAPoW(ctx, *job, miner, miner.Address())
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	for i, bc := range s.nodes {
		if err := bc.SubmitAPoW(*proof); err != nil {
			t.Fatalf("validator %d rejected common APoW proof: %v", i, err)
		}
	}

	done := func() bool {
		for _, bc := range s.nodes {
			if bc.FinalizedHeight() < 9 {
				return false
			}
		}
		return true
	}
	for n := 0; n < 12 && !done(); n++ {
		for _, r := range reactors {
			if err := r.HandleTimeout(); err != nil {
				t.Fatal(err)
			}
		}
		bus.drainUntil(40000, done)
	}
	if !done() {
		for i, r := range reactors {
			t.Logf("validator %d status=%+v finalized=%d", i, r.Status(), s.nodes[i].FinalizedHeight())
		}
		t.Fatal("height 9 did not finalize after APoW admission above round 64")
	}
	want := s.nodes[0].HeadSnapshot().Hash
	for i, bc := range s.nodes {
		b := bc.HeadSnapshot()
		if b.Index != 9 || bc.FinalizedHeight() != 9 || !strings.EqualFold(b.Hash, want) || b.APoW == nil {
			t.Fatalf("validator %d final state differs: height=%d finalized=%d hash=%s apow=%v", i, b.Index, bc.FinalizedHeight(), b.Hash, b.APoW != nil)
		}
	}
}
