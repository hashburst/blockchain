package blockchain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

type phase3DSetup struct {
	cfg   ProtocolV2Config
	vals  []sandboxValidator
	nodes []*Blockchain
	byID  map[string]sandboxValidator
}

func setupPhase3DChains(t *testing.T) phase3DSetup {
	t.Helper()
	cfg := phase3CTestConfig()
	cfg.ConsensusNetwork.ProposalTimeout = 20 * time.Millisecond
	cfg.ConsensusNetwork.PrevoteTimeout = 20 * time.Millisecond
	cfg.ConsensusNetwork.PrecommitTimeout = 20 * time.Millisecond
	cfg.ConsensusNetwork.RoundTimeoutDelta = time.Millisecond

	vals := make([]sandboxValidator, 4)
	byID := make(map[string]sandboxValidator, 4)
	for i := range vals {
		vals[i] = newSandboxValidator(t, i)
		byID[strings.ToLower(vals[i].id)] = vals[i]
	}
	nodes := make([]*Blockchain, 4)
	for i := range nodes {
		nodes[i] = NewBlockchainWithDirAndV2Config(t.TempDir(), cfg)
	}
	leader := nodes[0]
	for _, v := range vals {
		leader.SetPendingTransactions([]*Transaction{signedNodeRegistration(t, v, cfg.ChainID)}, nil)
		if err := leader.AddBlock(v.operator.Address()); err != nil {
			t.Fatalf("node identity bootstrap: %v", err)
		}
		b := leader.HeadSnapshot()
		for n := 1; n < len(nodes); n++ {
			if err := nodes[n].AppendBlock(cloneBlockForConsensus(b)); err != nil {
				t.Fatalf("append identity bootstrap to node %d: %v", n, err)
			}
		}
	}
	regs := make([]*protocolv2.TransactionV2, 0, 4)
	for _, v := range vals {
		regs = append(regs, validatorRegisterTx(t, cfg, v, 0))
	}
	leader.SetPendingTransactions(nil, regs)
	if err := leader.AddBlock(vals[0].operator.Address()); err != nil {
		t.Fatalf("validator registration block: %v", err)
	}
	b5 := leader.HeadSnapshot()
	for n := 1; n < len(nodes); n++ {
		if err := nodes[n].AppendBlock(cloneBlockForConsensus(b5)); err != nil {
			t.Fatalf("append validator registration to node %d: %v", n, err)
		}
	}
	leader.SetPendingTransactions(nil, nil)
	if err := leader.AddBlock(vals[0].operator.Address()); err != nil {
		t.Fatalf("validator activation block: %v", err)
	}
	b6 := leader.HeadSnapshot()
	for n := 1; n < len(nodes); n++ {
		if err := nodes[n].AppendBlock(cloneBlockForConsensus(b6)); err != nil {
			t.Fatalf("append validator activation to node %d: %v", n, err)
		}
	}
	return phase3DSetup{cfg: cfg, vals: vals, nodes: nodes, byID: byID}
}

// testConsensusBus deliberately queues delivery instead of invoking a peer
// synchronously from Broadcast. Real libp2p delivery is asynchronous and the
// reactor holds its safety mutex while emitting local messages.
type testConsensusBus struct {
	reactors []*ConsensusReactor
	active   []bool
	queue    []func()
}

type testConsensusEndpoint struct {
	bus  *testConsensusBus
	self int
}

func (b *testConsensusBus) enqueueExcept(self int, fn func(int)) {
	for i := range b.reactors {
		if i == self || !b.active[i] || b.reactors[i] == nil {
			continue
		}
		idx := i
		b.queue = append(b.queue, func() { fn(idx) })
	}
}

func (b *testConsensusBus) drain(limit int) {
	for len(b.queue) > 0 && limit > 0 {
		fn := b.queue[0]
		b.queue = b.queue[1:]
		fn()
		limit--
	}
}

func (b *testConsensusBus) drainUntil(limit int, done func() bool) {
	for len(b.queue) > 0 && limit > 0 {
		if done != nil && done() {
			return
		}
		fn := b.queue[0]
		b.queue = b.queue[1:]
		fn()
		limit--
	}
}

func (e *testConsensusEndpoint) BroadcastConsensusProposal(p ConsensusProposal) error {
	copyP := ConsensusProposal{Header: p.Header, Block: cloneBlockForConsensus(p.Block)}
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandleProposal(copyP) })
	return nil
}
func (e *testConsensusEndpoint) BroadcastConsensusPrevote(v consensus.Prevote) error {
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandlePrevote(v) })
	return nil
}
func (e *testConsensusEndpoint) BroadcastConsensusPrecommit(v consensus.Vote) error {
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandlePrecommit(v) })
	return nil
}
func (e *testConsensusEndpoint) BroadcastConsensusRoundChange(v consensus.RoundChange) error {
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandleRoundChange(v) })
	return nil
}
func (e *testConsensusEndpoint) BroadcastConsensusEvidence(v consensus.BFTDoubleSignEvidence) error {
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandleEvidence(v) })
	return nil
}
func (e *testConsensusEndpoint) BroadcastConsensusFinalized(v *Block) error {
	copyB := cloneBlockForConsensus(v)
	e.bus.enqueueExcept(e.self, func(i int) { _ = e.bus.reactors[i].HandleFinalizedBlock(copyB) })
	return nil
}

func attachPhase3DReactors(t *testing.T, s phase3DSetup, active []bool) (*testConsensusBus, []*ConsensusReactor) {
	t.Helper()
	reactors := make([]*ConsensusReactor, len(s.nodes))
	bus := &testConsensusBus{active: append([]bool(nil), active...), reactors: reactors}
	for i := range s.nodes {
		var signer *wallet.Wallet
		validatorID := ""
		if active[i] {
			signer = s.vals[i].consensus
			validatorID = s.vals[i].id
		}
		r, err := NewConsensusReactor(s.nodes[i], validatorID, signer, nil)
		if err != nil {
			t.Fatal(err)
		}
		reactors[i] = r
	}
	bus.reactors = reactors
	for i, r := range reactors {
		if active[i] {
			r.SetTransport(&testConsensusEndpoint{bus: bus, self: i})
		}
	}
	return bus, reactors
}

func TestPhase3DViewChangeWithRoundZeroProposerOffline(t *testing.T) {
	s := setupPhase3DChains(t)
	set := s.nodes[0].CurrentValidatorSet(7)
	round0, _ := set.Proposer(7, 0)
	offline := -1
	for i, v := range s.vals {
		if strings.EqualFold(v.id, round0.ID) {
			offline = i
		}
	}
	if offline < 0 {
		t.Fatal("round-0 proposer fixture not found")
	}
	active := []bool{true, true, true, true}
	active[offline] = false
	bus, reactors := attachPhase3DReactors(t, s, active)
	for i, r := range reactors {
		if active[i] {
			if err := r.Start(); err != nil {
				t.Fatalf("start reactor %d: %v", i, err)
			}
		}
	}
	// No round-0 proposal can exist. All three online validators timeout and
	// prevote nil. 3-of-4 then advances to round 1, whose proposer is different.
	for i, r := range reactors {
		if active[i] {
			if err := r.HandleTimeout(); err != nil {
				t.Fatalf("round-0 timeout reactor %d: %v", i, err)
			}
		}
	}
	bus.drainUntil(10000, func() bool {
		for i, bc := range s.nodes {
			if active[i] && bc.FinalizedHeight() < 7 {
				return false
			}
		}
		return true
	})
	for i, r := range reactors {
		if active[i] {
			r.Stop()
		}
	}

	var finalized *Block
	for i, bc := range s.nodes {
		if !active[i] {
			continue
		}
		if bc.FinalizedHeight() < 7 {
			t.Fatalf("online node %d did not finalize after view change: head=%d finalized=%d status=%+v", i, bc.Height(), bc.FinalizedHeight(), reactors[i].Status())
		}
		if len(bc.Blocks) <= 7 {
			t.Fatalf("online node %d finalized >=7 but has no height-7 block", i)
		}
		if finalized == nil {
			finalized = cloneBlockForConsensus(bc.Blocks[7])
		} else if !strings.EqualFold(finalized.Hash, bc.Blocks[7].Hash) {
			t.Fatalf("online nodes finalized different values")
		}
	}
	if finalized == nil || finalized.ConsensusRound == 0 {
		t.Fatalf("expected finality after a non-zero view, block=%+v", finalized)
	}
	// The offline node can later catch up from the signed finalized block without
	// participating in the round that produced it.
	if err := reactors[offline].HandleFinalizedBlock(finalized); err != nil {
		t.Fatalf("offline node rejected finalized catch-up block: %v", err)
	}
	if s.nodes[offline].FinalizedHeight() != 7 {
		t.Fatal("offline node did not catch up to finalized height")
	}
}

func TestPhase3DTwoOfFourCannotFinalize(t *testing.T) {
	s := setupPhase3DChains(t)
	active := []bool{true, true, false, false}
	bus, reactors := attachPhase3DReactors(t, s, active)
	for i, r := range reactors {
		if active[i] {
			if err := r.Start(); err != nil {
				t.Fatal(err)
			}
		}
	}
	bus.drain(1000)
	for i, r := range reactors {
		if active[i] {
			_ = r.HandleTimeout()
		}
	}
	bus.drain(1000)
	for i, r := range reactors {
		if active[i] {
			_ = r.HandleTimeout()
		}
	}
	bus.drain(1000)
	for i, bc := range s.nodes {
		if active[i] && bc.FinalizedHeight() >= 7 {
			t.Fatalf("2-of-4 node %d unexpectedly finalized height 7", i)
		}
	}
}

func TestPhase3DReproposalPreservesContentHashAndRewardAuthor(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	set := bc.CurrentValidatorSet(7)
	p0, _ := set.Proposer(7, 0)
	block, err := bc.BuildConsensusProposal(p0.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	prevotes := make([]consensus.Prevote, 0, 3)
	for i := 0; i < 3; i++ {
		v := set.Validators[i]
		fixture := s.byID[strings.ToLower(v.ID)]
		pv, err := s.nodes[i].SignConsensusPrevote(7, 0, block.Hash, set.Root(), v.ID, fixture.consensus)
		if err != nil {
			t.Fatal(err)
		}
		prevotes = append(prevotes, pv)
	}
	qc, err := consensus.BuildPrevoteCertificate(s.cfg.ChainID, 7, 0, block.Hash, set.Root(), set, prevotes)
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := set.Proposer(7, 1)
	reproposal, err := bc.ReproposeConsensusValue(p1.ID, 1, block, 0, qc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(reproposal.Hash, block.Hash) {
		t.Fatalf("view change changed content hash: %s != %s", reproposal.Hash, block.Hash)
	}
	if !strings.EqualFold(reproposal.AuthorValidatorID, block.AuthorValidatorID) || !strings.EqualFold(reproposal.ProposerID, p1.ID) || reproposal.ConsensusRound != 1 {
		t.Fatalf("reproposal metadata incorrect: %+v", reproposal)
	}
	author, _, _ := set.Find(block.AuthorValidatorID)
	if err := validateRewardRecipient(reproposal, author.RewardAddress); err != nil {
		t.Fatalf("reproposal rewrote reward ownership: %v", err)
	}
}

func TestPhase3DConsensusFrameBounded(t *testing.T) {
	n := &Libp2pConsensusNetwork{cfg: consensus.DefaultNetworkConfig()}
	var buf strings.Builder
	payload := []byte(`{"type":"round_change"}`)
	if err := n.writePayload(&buf, payload); err != nil {
		t.Fatal(err)
	}
	got, err := n.readPayload(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("consensus frame roundtrip mismatch")
	}
	n.cfg.MaxMessageBytes = 8
	if err := n.writePayload(&buf, payload); err == nil {
		t.Fatal("oversized consensus frame unexpectedly accepted")
	}
	if _, err := n.readPayload(strings.NewReader(string([]byte{0, 0, 0, 9}))); err == nil {
		t.Fatal("oversized inbound consensus frame unexpectedly accepted")
	}
}

func TestPhase3DBFTEvidenceTransactionSlashesAndJails(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	set := bc.CurrentValidatorSet(7)
	if len(set.Validators) != 4 {
		t.Fatalf("validator set size=%d want 4", len(set.Validators))
	}

	offender := set.Validators[1]
	offenderFixture := s.byID[strings.ToLower(offender.ID)]
	voteA, err := consensus.NewSignedPrevote(s.cfg.ChainID, 7, 0, strings.Repeat("11", 32), set.Root(), offender.ID, offenderFixture.consensus)
	if err != nil {
		t.Fatal(err)
	}
	voteB, err := consensus.NewSignedPrevote(s.cfg.ChainID, 7, 0, strings.Repeat("22", 32), set.Root(), offender.ID, offenderFixture.consensus)
	if err != nil {
		t.Fatal(err)
	}
	evidence := consensus.BFTDoubleSignEvidence{
		Step: consensus.StepPrevote,
		Prevote: &consensus.PrevoteEquivocationEvidence{
			VoteA: voteA,
			VoteB: voteB,
		},
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}

	reporter := s.vals[0].operator
	fee, err := s.cfg.FeePolicy.ComputeFee(computeValidatorEvidence)
	if err != nil {
		t.Fatal(err)
	}
	tx := protocolv2.NewTransactionV2(
		s.cfg.ChainID,
		protocolv2.TxValidatorEvidence,
		reporter.Address(),
		"",
		0,
		bc.state.Sequence(reporter.Address()),
		computeValidatorEvidence,
		fee,
		data,
	)
	if err := tx.Sign(reporter); err != nil {
		t.Fatal(err)
	}
	if err := bc.validateV2Transaction(tx); err != nil {
		t.Fatalf("validate BFT evidence transaction: %v", err)
	}
	bc.SetPendingTransactions(nil, []*protocolv2.TransactionV2{tx})

	proposer, ok := set.Proposer(7, 1)
	if !ok {
		t.Fatal("round-1 proposer unavailable")
	}
	proposal, err := bc.BuildConsensusProposal(proposer.ID, 1)
	if err != nil {
		t.Fatalf("build evidence proposal: %v", err)
	}
	votes := make([]consensus.Vote, 0, 3)
	for i := 0; i < 3; i++ {
		v := set.Validators[i]
		fixture := s.byID[strings.ToLower(v.ID)]
		vote, err := consensus.NewSignedVote(s.cfg.ChainID, 7, 1, proposal.Hash, proposal.ValidatorSetRoot, v.ID, fixture.consensus)
		if err != nil {
			t.Fatal(err)
		}
		votes = append(votes, vote)
	}
	qc, err := consensus.BuildQuorumCertificate(s.cfg.ChainID, 7, 1, proposal.Hash, proposal.ValidatorSetRoot, set, votes)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := bc.ValidatorRegistry().Get(offender.ID)
	if !ok {
		t.Fatal("offender missing before evidence")
	}
	if err := bc.FinalizeConsensusProposal(proposal, qc); err != nil {
		t.Fatalf("finalize evidence proposal: %v", err)
	}
	after, ok := bc.ValidatorRegistry().Get(offender.ID)
	if !ok {
		t.Fatal("offender missing after evidence")
	}
	if after.Status != consensus.ValidatorJailed {
		t.Fatalf("offender status=%s want %s", after.Status, consensus.ValidatorJailed)
	}
	if after.BondUnits >= before.BondUnits || after.SlashedUnits <= before.SlashedUnits {
		t.Fatalf("slash not applied: before=%d/%d after=%d/%d", before.BondUnits, before.SlashedUnits, after.BondUnits, after.SlashedUnits)
	}
	if bc.BalanceUnits(s.cfg.SlashCollector) <= 0 {
		t.Fatal("slash collector did not receive HBT")
	}
	receipt, ok := bc.Receipt(tx.HashHex())
	if !ok || !receipt.Success {
		t.Fatalf("BFT evidence receipt missing/failed: %+v", receipt)
	}
}

func TestPhase3DOutOfOrderPrecommitQCWaitsForValidatedProposal(t *testing.T) {
	s := setupPhase3DChains(t)
	bc := s.nodes[0]
	set := bc.CurrentValidatorSet(7)
	proposer, ok := set.Proposer(7, 0)
	if !ok {
		t.Fatal("proposer unavailable")
	}
	proposalBlock, err := bc.BuildConsensusProposal(proposer.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	fixture := s.byID[strings.ToLower(proposer.ID)]
	header, err := bc.SignConsensusProposalHeader(proposalBlock, proposer.ID, fixture.consensus)
	if err != nil {
		t.Fatal(err)
	}
	proposal := ConsensusProposal{Header: header, Block: proposalBlock}

	reactor, err := NewConsensusReactor(bc, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reactor.Start(); err != nil {
		t.Fatal(err)
	}
	defer reactor.Stop()

	// Deliver a complete authenticated precommit quorum before the proposal.
	// The reactor must retain the QC but cannot finalize unvalidated block data.
	for i := 0; i < 3; i++ {
		v := set.Validators[i]
		signer := s.byID[strings.ToLower(v.ID)].consensus
		vote, err := consensus.NewSignedVote(s.cfg.ChainID, 7, 0, proposalBlock.Hash, proposalBlock.ValidatorSetRoot, v.ID, signer)
		if err != nil {
			t.Fatal(err)
		}
		if err := reactor.HandlePrecommit(vote); err != nil {
			t.Fatalf("out-of-order precommit %d rejected: %v", i, err)
		}
	}
	if bc.FinalizedHeight() >= 7 {
		t.Fatal("QC without proposal data unexpectedly finalized block")
	}

	if err := reactor.HandleProposal(proposal); err != nil {
		t.Fatalf("validated delayed proposal rejected: %v", err)
	}
	if bc.FinalizedHeight() != 7 {
		t.Fatalf("delayed proposal did not unlock retained QC: finalized=%d", bc.FinalizedHeight())
	}
	if !strings.EqualFold(bc.HeadSnapshot().Hash, proposalBlock.Hash) {
		t.Fatal("finalized hash differs from delayed proposal")
	}
}
