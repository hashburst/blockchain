package blockchain

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hashburst/consensus"
	"hashburst/wallet"
)

// ConsensusProposal is the signed Phase 3D network envelope. The block hash is
// a content hash: current proposer/round and certificates are metadata and do
// not change it during a safe view change.
type ConsensusProposal struct {
	Header consensus.ProposalHeader `json:"header"`
	Block  *Block                   `json:"block"`
}

type ConsensusTransport interface {
	BroadcastConsensusProposal(ConsensusProposal) error
	BroadcastConsensusPrevote(consensus.Prevote) error
	BroadcastConsensusPrecommit(consensus.Vote) error
	BroadcastConsensusRoundChange(consensus.RoundChange) error
	BroadcastConsensusEvidence(consensus.BFTDoubleSignEvidence) error
	BroadcastConsensusFinalized(*Block) error
}

type ConsensusReactorStatus struct {
	Height           uint64            `json:"height"`
	Round            uint64            `json:"round"`
	Step             consensus.BFTStep `json:"step"`
	LockedRound      int64             `json:"locked_round"`
	LockedBlockHash  string            `json:"locked_block_hash,omitempty"`
	ValidRound       int64             `json:"valid_round"`
	ValidBlockHash   string            `json:"valid_block_hash,omitempty"`
	LocalValidatorID string            `json:"local_validator_id,omitempty"`
	Deadline         time.Time         `json:"deadline"`
	EvidenceCount    int               `json:"evidence_count"`
	Running          bool              `json:"running"`
}

type ConsensusReactor struct {
	mu sync.Mutex

	bc        *Blockchain
	cfg       consensus.NetworkConfig
	transport ConsensusTransport

	validatorID string
	signer      *wallet.Wallet

	height uint64
	round  uint64
	step   consensus.BFTStep

	lockedRound int64
	lockedHash  string
	lockedBlock *Block
	validRound  int64
	validHash   string
	validBlock  *Block
	validQC     *consensus.PrevoteCertificate

	proposals      map[uint64]ConsensusProposal
	blocksByHash   map[string]*Block
	prevotes       map[uint64]map[string]map[string]consensus.Prevote
	precommits     map[uint64]map[string]map[string]consensus.Vote
	prevoteQCs     map[uint64]map[string]consensus.PrevoteCertificate
	precommitQCs   map[uint64]map[string]consensus.QuorumCertificate
	seenPrevotes   map[uint64]map[string]consensus.Prevote
	seenPrecommits map[uint64]map[string]consensus.Vote
	roundChanges   map[uint64]map[string]consensus.RoundChange
	evidence       map[string]consensus.BFTDoubleSignEvidence

	deadline     time.Time
	running      bool
	runningState atomic.Bool
}

func NewConsensusReactor(bc *Blockchain, validatorID string, signer *wallet.Wallet, transport ConsensusTransport) (*ConsensusReactor, error) {
	if bc == nil {
		return nil, fmt.Errorf("nil blockchain")
	}
	cfg := bc.v2Config.ConsensusNetwork
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if (strings.TrimSpace(validatorID) == "") != (signer == nil) {
		return nil, fmt.Errorf("local validator id and signer must be configured together")
	}
	r := &ConsensusReactor{bc: bc, cfg: cfg, transport: transport, validatorID: strings.TrimSpace(validatorID), signer: signer}
	r.resetMapsLocked()
	r.lockedRound, r.validRound = -1, -1
	bc.attachConsensusReactor(r)
	return r, nil
}

func (r *ConsensusReactor) resetMapsLocked() {
	r.proposals = make(map[uint64]ConsensusProposal)
	r.blocksByHash = make(map[string]*Block)
	r.prevotes = make(map[uint64]map[string]map[string]consensus.Prevote)
	r.precommits = make(map[uint64]map[string]map[string]consensus.Vote)
	r.prevoteQCs = make(map[uint64]map[string]consensus.PrevoteCertificate)
	r.precommitQCs = make(map[uint64]map[string]consensus.QuorumCertificate)
	r.seenPrevotes = make(map[uint64]map[string]consensus.Prevote)
	r.seenPrecommits = make(map[uint64]map[string]consensus.Vote)
	r.roundChanges = make(map[uint64]map[string]consensus.RoundChange)
	r.evidence = make(map[string]consensus.BFTDoubleSignEvidence)
}

func (r *ConsensusReactor) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		r.runningState.Store(true)
		return nil
	}
	height := uint64(r.bc.Height() + 1)
	if !r.bc.v2Config.ConsensusEnabledAt(int(height)) {
		return fmt.Errorf("consensus network protocol disabled at height %d", height)
	}
	r.running = true
	r.runningState.Store(true)
	var err error
	if r.height != 0 && r.height == height {
		// Administrative resume retains locks, certified values and received
		// votes. Enter a fresh view through the normal journaled transition.
		if r.round >= r.cfg.MaxRound {
			err = fmt.Errorf("cannot resume consensus: exhausted max round %d", r.cfg.MaxRound)
		} else {
			err = r.requestRoundChangeLocked(r.round + 1)
		}
	} else {
		err = r.startHeightLocked(height)
	}
	if err != nil {
		r.running = false
		r.runningState.Store(false)
		r.deadline = time.Time{}
		return err
	}
	return nil
}

func (r *ConsensusReactor) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.runningState.Store(false)
	r.deadline = time.Time{}
}

// Running reports the reactor lifecycle state without contending on the BFT
// state mutex. It is intended for health/control-plane checks; consensus state
// transitions continue to use r.mu.
func (r *ConsensusReactor) Running() bool {
	return r.runningState.Load()
}

// SetTransport attaches or replaces the network transport. It exists to break
// the constructor cycle between the reactor and the libp2p transport. Attaching
// a transport does not start consensus or alter chain state.
func (r *ConsensusReactor) SetTransport(transport ConsensusTransport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transport = transport
}

func (r *ConsensusReactor) Run(ctx context.Context) error {
	if err := r.Start(); err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.Stop()
			return ctx.Err()
		case now := <-ticker.C:
			r.mu.Lock()
			due := r.running && !r.deadline.IsZero() && !now.Before(r.deadline)
			r.mu.Unlock()
			if due {
				_ = r.HandleTimeout()
			}
		}
	}
}

func (r *ConsensusReactor) statusLocked() ConsensusReactorStatus {
	return ConsensusReactorStatus{Height: r.height, Round: r.round, Step: r.step, LockedRound: r.lockedRound, LockedBlockHash: r.lockedHash, ValidRound: r.validRound, ValidBlockHash: r.validHash, LocalValidatorID: r.validatorID, Deadline: r.deadline, EvidenceCount: len(r.evidence), Running: r.running}
}

func (r *ConsensusReactor) Status() ConsensusReactorStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

// TryStatus returns a fresh detailed status only when doing so cannot block the
// control plane behind consensus work. Running() remains authoritative for the
// lifecycle bit when the detailed mutex is busy.
func (r *ConsensusReactor) TryStatus() (ConsensusReactorStatus, bool) {
	if !r.mu.TryLock() {
		return ConsensusReactorStatus{Running: r.Running()}, false
	}
	defer r.mu.Unlock()
	return r.statusLocked(), true
}

// SyncToFinalizedHead advances a running reactor after the blockchain accepted
// a QC-finalized block through chain sync rather than through HandleFinalizedBlock.
// It never rewinds the reactor and ignores pre-consensus/unfinalized heads.
func (r *ConsensusReactor) SyncToFinalizedHead(head *Block) error {
	if head == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || !r.bc.v2Config.ConsensusEnabledAt(head.Index) || head.FinalityCertificate == nil {
		return nil
	}
	next := uint64(head.Index + 1)
	if next <= r.height {
		return nil
	}
	return r.startHeightLocked(next)
}

func (r *ConsensusReactor) PendingEvidence() []consensus.BFTDoubleSignEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]consensus.BFTDoubleSignEvidence, 0, len(r.evidence))
	for _, ev := range r.evidence {
		out = append(out, ev)
	}
	return out
}

func (r *ConsensusReactor) startHeightLocked(height uint64) error {
	r.height = height
	r.round = 0
	r.step = consensus.StepProposal
	r.lockedRound, r.validRound = -1, -1
	r.lockedHash, r.validHash = "", ""
	r.lockedBlock, r.validBlock, r.validQC = nil, nil, nil
	r.resetMapsLocked()
	return r.enterRoundLocked(0)
}

func (r *ConsensusReactor) enterRoundLocked(round uint64) error {
	if round > r.cfg.MaxRound {
		return fmt.Errorf("consensus round %d exceeds max %d", round, r.cfg.MaxRound)
	}
	if round < r.round {
		return nil
	}
	r.round = round
	r.step = consensus.StepProposal
	r.deadline = time.Now().Add(r.cfg.TimeoutFor(consensus.StepProposal, round))

	// A proposal may have arrived before f+1 round-change messages let us catch
	// up. Process it now that the pacemaker has entered the round.
	if p, ok := r.proposals[round]; ok {
		return r.acceptProposalLocked(p, true)
	}

	set := r.bc.CurrentValidatorSet(r.height)
	expected, ok := set.Proposer(r.height, round)
	if !ok || r.signer == nil || !strings.EqualFold(expected.ID, r.validatorID) {
		return nil
	}

	// Building a proposal re-executes all native/HVM state transitions. That work
	// must not hold the reactor mutex: network handlers and the pacemaker need to
	// remain able to advance the round while an expensive proposal is being built.
	// Capture only immutable/snapshotted inputs, release the lock, then discard the
	// result if height/round/step changed before the build completed.
	height := r.height
	validatorID := r.validatorID
	validRound := r.validRound
	validBlock := cloneBlockForConsensus(r.validBlock)
	validQC := clonePrevoteQC(r.validQC)

	r.mu.Unlock()
	var block *Block
	var err error
	if validBlock != nil && validQC != nil && validRound >= 0 && uint64(validRound) < round {
		block, err = r.bc.ReproposeConsensusValue(validatorID, round, validBlock, uint64(validRound), *validQC)
	} else {
		block, err = r.bc.BuildConsensusProposal(validatorID, round)
	}
	r.mu.Lock()

	if !r.running || r.height != height || r.round != round || r.step != consensus.StepProposal {
		// The build became stale while the reactor continued processing timeouts or
		// network traffic. Never sign or broadcast a proposal for an obsolete view.
		return nil
	}
	if p, ok := r.proposals[round]; ok {
		// A valid network proposal won the race while the local value was building.
		return r.acceptProposalLocked(p, true)
	}
	if err != nil {
		return err
	}

	header, err := r.bc.SignConsensusProposalHeader(block, validatorID, r.signer)
	if err != nil {
		return err
	}
	proposal := ConsensusProposal{Header: header, Block: block}
	r.proposals[round] = proposal
	r.blocksByHash[strings.ToLower(block.Hash)] = cloneBlockForConsensus(block)
	if r.transport != nil {
		_ = r.transport.BroadcastConsensusProposal(proposal)
	}
	return r.acceptProposalLocked(proposal, true)
}

func (r *ConsensusReactor) HandleProposal(p ConsensusProposal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return fmt.Errorf("consensus reactor not running")
	}
	return r.acceptProposalLocked(p, p.Header.Round == r.round)
}

func (r *ConsensusReactor) acceptProposalLocked(p ConsensusProposal, mayVote bool) error {
	if p.Block == nil {
		return fmt.Errorf("nil consensus proposal block")
	}
	h := p.Header
	if h.ChainID != r.bc.v2Config.ChainID || h.Height != uint64(p.Block.Index) || h.Height != r.height || h.Round != p.Block.ConsensusRound || !strings.EqualFold(h.BlockHash, p.Block.Hash) || !strings.EqualFold(h.ValidatorSetRoot, p.Block.ValidatorSetRoot) || !strings.EqualFold(h.ProposerID, p.Block.ProposerID) || h.ValidRound != p.Block.ValidRound {
		return fmt.Errorf("proposal header/block mismatch")
	}
	if h.Round > r.round+r.cfg.MaxRound || h.Round > r.cfg.MaxRound {
		return fmt.Errorf("proposal round out of bounds")
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if !strings.EqualFold(set.Root(), h.ValidatorSetRoot) {
		return fmt.Errorf("proposal validator set root mismatch")
	}
	expected, ok := set.Proposer(r.height, h.Round)
	if !ok || !strings.EqualFold(expected.ID, h.ProposerID) {
		return fmt.Errorf("proposal not signed by scheduled proposer")
	}
	if err := consensus.VerifyProposalHeader(h, expected); err != nil {
		return err
	}
	if err := r.bc.ValidateConsensusProposalForVote(p.Block); err != nil {
		return fmt.Errorf("proposal validation: %w", err)
	}
	safeErr := consensus.SafeProposal(r.lockedRound, r.lockedHash, p.Block.Hash, p.Block.ValidRound, p.Block.ValidPrevoteCertificate, set)
	if safeErr != nil {
		// A proposal can arrive after this node already observed a +2/3 prevote
		// certificate for it in the same round. That certificate is stronger than
		// the proposal envelope itself and safely justifies moving a prior lock.
		if qc, ok := r.lookupPrevoteQCLocked(h.Round, p.Block.Hash); ok && int64(qc.Round) >= r.lockedRound {
			copyQC := qc
			if err := copyQC.Verify(set); err == nil && strings.EqualFold(copyQC.BlockHash, p.Block.Hash) {
				safeErr = nil
			}
		}
	}
	if safeErr != nil {
		return fmt.Errorf("proposal violates local lock: %w", safeErr)
	}
	r.proposals[h.Round] = ConsensusProposal{Header: h, Block: cloneBlockForConsensus(p.Block)}
	r.blocksByHash[strings.ToLower(p.Block.Hash)] = cloneBlockForConsensus(p.Block)

	// Network messages can be reordered. If a QC arrived before the block, the
	// newly validated proposal completes the missing dependency immediately.
	if qc, ok := r.lookupPrecommitQCLocked(h.Round, p.Block.Hash); ok {
		return r.finalizeWithQCLocked(qc)
	}
	if qc, ok := r.lookupPrevoteQCLocked(h.Round, p.Block.Hash); ok {
		if err := r.applyPrevoteQCLocked(qc); err != nil {
			return err
		}
		// applyPrevoteQCLocked may already have moved this node to precommit.
		if r.step == consensus.StepPrecommit || !mayVote || h.Round != r.round {
			return nil
		}
	}
	if !mayVote || h.Round != r.round {
		return nil
	}
	if r.step != consensus.StepProposal {
		return nil
	}
	r.step = consensus.StepPrevote
	r.deadline = time.Now().Add(r.cfg.TimeoutFor(consensus.StepPrevote, r.round))
	return r.signAndBroadcastPrevoteLocked(p.Block.Hash)
}

func (r *ConsensusReactor) signAndBroadcastPrevoteLocked(blockHash string) error {
	if r.signer == nil {
		return nil
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if _, _, ok := set.Find(r.validatorID); !ok {
		return nil
	}
	if seen := r.seenPrevotes[r.round]; seen != nil {
		if _, exists := seen[strings.ToLower(r.validatorID)]; exists {
			return nil
		}
	}
	pv, err := r.bc.SignConsensusPrevote(r.height, r.round, blockHash, set.Root(), r.validatorID, r.signer)
	if err != nil {
		return err
	}
	if err := r.acceptPrevoteLocked(pv); err != nil {
		return err
	}
	if r.transport != nil {
		_ = r.transport.BroadcastConsensusPrevote(pv)
	}
	return nil
}

func (r *ConsensusReactor) HandlePrevote(v consensus.Prevote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return fmt.Errorf("consensus reactor not running")
	}
	return r.acceptPrevoteLocked(v)
}

func (r *ConsensusReactor) acceptPrevoteLocked(v consensus.Prevote) error {
	if v.ChainID != r.bc.v2Config.ChainID || v.Height != r.height || v.Round > r.cfg.MaxRound {
		return fmt.Errorf("prevote outside current consensus window")
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if !strings.EqualFold(v.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("prevote validator set mismatch")
	}
	validator, _, ok := set.Find(v.ValidatorID)
	if !ok {
		return fmt.Errorf("prevote validator not active")
	}
	if err := consensus.VerifyPrevote(v, validator); err != nil {
		return err
	}
	id := strings.ToLower(v.ValidatorID)
	if r.seenPrevotes[v.Round] == nil {
		r.seenPrevotes[v.Round] = make(map[string]consensus.Prevote)
	}
	if old, exists := r.seenPrevotes[v.Round][id]; exists {
		if strings.EqualFold(old.BlockHash, v.BlockHash) {
			return nil
		}
		ev := consensus.BFTDoubleSignEvidence{Step: consensus.StepPrevote, Prevote: &consensus.PrevoteEquivocationEvidence{VoteA: old, VoteB: v}}
		r.recordEvidenceLocked(ev)
		return fmt.Errorf("validator %s equivocated in prevote round %d", v.ValidatorID, v.Round)
	}
	r.seenPrevotes[v.Round][id] = v
	hash := strings.ToLower(v.BlockHash)
	if r.prevotes[v.Round] == nil {
		r.prevotes[v.Round] = make(map[string]map[string]consensus.Prevote)
	}
	if r.prevotes[v.Round][hash] == nil {
		r.prevotes[v.Round][hash] = make(map[string]consensus.Prevote)
	}
	r.prevotes[v.Round][hash][id] = v
	votes := mapPrevotes(r.prevotes[v.Round][hash])
	if votePowerPrevotes(set, votes) < consensus.QuorumThreshold(set.TotalPower()) {
		return nil
	}
	if strings.EqualFold(v.BlockHash, consensus.NilBlockHash) || strings.TrimSpace(v.BlockHash) == "" {
		if v.Round == r.round {
			return r.requestRoundChangeLocked(r.round + 1)
		}
		return nil
	}
	qc, err := consensus.BuildPrevoteCertificate(r.bc.v2Config.ChainID, r.height, v.Round, v.BlockHash, set.Root(), set, votes)
	if err != nil {
		return err
	}
	r.storePrevoteQCLocked(qc)
	return r.applyPrevoteQCLocked(qc)
}

func (r *ConsensusReactor) storePrevoteQCLocked(qc consensus.PrevoteCertificate) {
	if r.prevoteQCs[qc.Round] == nil {
		r.prevoteQCs[qc.Round] = make(map[string]consensus.PrevoteCertificate)
	}
	r.prevoteQCs[qc.Round][strings.ToLower(qc.BlockHash)] = qc
}

func (r *ConsensusReactor) lookupPrevoteQCLocked(round uint64, blockHash string) (consensus.PrevoteCertificate, bool) {
	byHash := r.prevoteQCs[round]
	if byHash == nil {
		return consensus.PrevoteCertificate{}, false
	}
	qc, ok := byHash[strings.ToLower(blockHash)]
	return qc, ok
}

func (r *ConsensusReactor) applyPrevoteQCLocked(qc consensus.PrevoteCertificate) error {
	if strings.EqualFold(qc.BlockHash, consensus.NilBlockHash) {
		return nil
	}
	proposal, ok := r.proposals[qc.Round]
	if !ok || proposal.Block == nil || !strings.EqualFold(proposal.Block.Hash, qc.BlockHash) {
		// Keep the highest observed valid QC, but never sign a precommit without
		// having independently validated the full proposal block.
		if int64(qc.Round) >= r.validRound {
			r.validRound, r.validHash, r.validQC = int64(qc.Round), qc.BlockHash, clonePrevoteQC(&qc)
		}
		return nil
	}
	if int64(qc.Round) >= r.validRound {
		r.validRound, r.validHash, r.validBlock, r.validQC = int64(qc.Round), qc.BlockHash, cloneBlockForConsensus(proposal.Block), clonePrevoteQC(&qc)
	}
	if int64(qc.Round) >= r.lockedRound {
		r.lockedRound, r.lockedHash, r.lockedBlock = int64(qc.Round), qc.BlockHash, cloneBlockForConsensus(proposal.Block)
	}
	if qc.Round != r.round {
		return nil
	}
	r.step = consensus.StepPrecommit
	r.deadline = time.Now().Add(r.cfg.TimeoutFor(consensus.StepPrecommit, r.round))
	return r.signAndBroadcastPrecommitLocked(proposal.Block)
}

func mapPrevotes(m map[string]consensus.Prevote) []consensus.Prevote {
	out := make([]consensus.Prevote, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func votePowerPrevotes(set consensus.ValidatorSet, votes []consensus.Prevote) uint64 {
	seen := make(map[string]struct{})
	var power uint64
	for _, v := range votes {
		id := strings.ToLower(v.ValidatorID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		_, p, ok := set.Find(v.ValidatorID)
		if ok {
			power += p
		}
	}
	return power
}

func (r *ConsensusReactor) signAndBroadcastPrecommitLocked(block *Block) error {
	if r.signer == nil || block == nil {
		return nil
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if _, _, ok := set.Find(r.validatorID); !ok {
		return nil
	}
	if seen := r.seenPrecommits[r.round]; seen != nil {
		if _, exists := seen[strings.ToLower(r.validatorID)]; exists {
			return nil
		}
	}
	vote, err := r.bc.SignConsensusPrecommit(block, r.validatorID, r.signer)
	if err != nil {
		return err
	}
	if err := r.acceptPrecommitLocked(vote); err != nil {
		return err
	}
	if r.transport != nil {
		_ = r.transport.BroadcastConsensusPrecommit(vote)
	}
	return nil
}

func (r *ConsensusReactor) HandlePrecommit(v consensus.Vote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return fmt.Errorf("consensus reactor not running")
	}
	return r.acceptPrecommitLocked(v)
}

func (r *ConsensusReactor) acceptPrecommitLocked(v consensus.Vote) error {
	if v.ChainID != r.bc.v2Config.ChainID || v.Height != r.height || v.Round > r.cfg.MaxRound {
		return fmt.Errorf("precommit outside current consensus window")
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if !strings.EqualFold(v.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("precommit validator set mismatch")
	}
	validator, _, ok := set.Find(v.ValidatorID)
	if !ok {
		return fmt.Errorf("precommit validator not active")
	}
	if err := consensus.VerifyVote(v, validator); err != nil {
		return err
	}
	id := strings.ToLower(v.ValidatorID)
	if r.seenPrecommits[v.Round] == nil {
		r.seenPrecommits[v.Round] = make(map[string]consensus.Vote)
	}
	if old, exists := r.seenPrecommits[v.Round][id]; exists {
		if strings.EqualFold(old.BlockHash, v.BlockHash) {
			return nil
		}
		ev := consensus.BFTDoubleSignEvidence{Step: consensus.StepPrecommit, Precommit: &consensus.PrecommitEquivocationEvidence{VoteA: old, VoteB: v}}
		r.recordEvidenceLocked(ev)
		return fmt.Errorf("validator %s equivocated in precommit round %d", v.ValidatorID, v.Round)
	}
	r.seenPrecommits[v.Round][id] = v
	hash := strings.ToLower(v.BlockHash)
	if r.precommits[v.Round] == nil {
		r.precommits[v.Round] = make(map[string]map[string]consensus.Vote)
	}
	if r.precommits[v.Round][hash] == nil {
		r.precommits[v.Round][hash] = make(map[string]consensus.Vote)
	}
	r.precommits[v.Round][hash][id] = v
	votes := mapPrecommits(r.precommits[v.Round][hash])
	if votePowerPrecommits(set, votes) < consensus.QuorumThreshold(set.TotalPower()) {
		return nil
	}
	qc, err := consensus.BuildQuorumCertificate(r.bc.v2Config.ChainID, r.height, v.Round, v.BlockHash, set.Root(), set, votes)
	if err != nil {
		return err
	}
	r.storePrecommitQCLocked(qc)
	return r.finalizeWithQCLocked(qc)
}

func (r *ConsensusReactor) storePrecommitQCLocked(qc consensus.QuorumCertificate) {
	if r.precommitQCs[qc.Round] == nil {
		r.precommitQCs[qc.Round] = make(map[string]consensus.QuorumCertificate)
	}
	r.precommitQCs[qc.Round][strings.ToLower(qc.BlockHash)] = qc
}

func (r *ConsensusReactor) lookupPrecommitQCLocked(round uint64, blockHash string) (consensus.QuorumCertificate, bool) {
	byHash := r.precommitQCs[round]
	if byHash == nil {
		return consensus.QuorumCertificate{}, false
	}
	qc, ok := byHash[strings.ToLower(blockHash)]
	return qc, ok
}

func (r *ConsensusReactor) finalizeWithQCLocked(qc consensus.QuorumCertificate) error {
	proposal, ok := r.proposals[qc.Round]
	if !ok || proposal.Block == nil || !strings.EqualFold(proposal.Block.Hash, qc.BlockHash) {
		// Reordered delivery is normal: retain the authenticated QC and wait for
		// the corresponding proposal/finalized block rather than treating it as a
		// consensus failure.
		return nil
	}
	finalBlock := cloneBlockForConsensus(proposal.Block)
	if err := r.bc.FinalizeConsensusProposal(finalBlock, qc); err != nil {
		return err
	}
	head := r.bc.HeadSnapshot()
	if head == nil {
		return fmt.Errorf("consensus finalized block but chain head is unavailable")
	}
	if r.transport != nil {
		_ = r.transport.BroadcastConsensusFinalized(head)
	}
	return r.startHeightLocked(uint64(head.Index + 1))
}

func mapPrecommits(m map[string]consensus.Vote) []consensus.Vote {
	out := make([]consensus.Vote, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func votePowerPrecommits(set consensus.ValidatorSet, votes []consensus.Vote) uint64 {
	seen := make(map[string]struct{})
	var power uint64
	for _, v := range votes {
		id := strings.ToLower(v.ValidatorID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		_, p, ok := set.Find(v.ValidatorID)
		if ok {
			power += p
		}
	}
	return power
}

func (r *ConsensusReactor) HandleRoundChange(rc consensus.RoundChange) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return fmt.Errorf("consensus reactor not running")
	}
	if rc.ChainID != r.bc.v2Config.ChainID || rc.Height != r.height || rc.NextRound > r.cfg.MaxRound {
		return fmt.Errorf("round-change outside current consensus window")
	}
	set := r.bc.CurrentValidatorSet(r.height)
	if !strings.EqualFold(rc.ValidatorSetRoot, set.Root()) {
		return fmt.Errorf("round-change validator set mismatch")
	}
	validator, _, ok := set.Find(rc.ValidatorID)
	if !ok {
		return fmt.Errorf("round-change validator not active")
	}
	if err := consensus.VerifyRoundChange(rc, validator); err != nil {
		return err
	}
	if r.roundChanges[rc.NextRound] == nil {
		r.roundChanges[rc.NextRound] = make(map[string]consensus.RoundChange)
	}
	r.roundChanges[rc.NextRound][strings.ToLower(rc.ValidatorID)] = rc
	var power uint64
	for _, c := range r.roundChanges[rc.NextRound] {
		_, p, ok := set.Find(c.ValidatorID)
		if ok {
			power += p
		}
	}
	if rc.NextRound > r.round && power >= consensus.CatchupThreshold(set.TotalPower()) {
		// f+1 only advances the pacemaker; it is not a finality certificate.
		return r.enterRoundLocked(rc.NextRound)
	}
	return nil
}

func (r *ConsensusReactor) requestRoundChangeLocked(nextRound uint64) error {
	if nextRound > r.cfg.MaxRound {
		return fmt.Errorf("consensus exhausted max round %d", r.cfg.MaxRound)
	}
	if r.signer != nil {
		set := r.bc.CurrentValidatorSet(r.height)
		if _, _, ok := set.Find(r.validatorID); ok {
			rc, err := r.bc.SignConsensusRoundChange(r.height, nextRound, r.validatorID, r.signer)
			if err != nil {
				return err
			}
			if r.roundChanges[nextRound] == nil {
				r.roundChanges[nextRound] = make(map[string]consensus.RoundChange)
			}
			r.roundChanges[nextRound][strings.ToLower(rc.ValidatorID)] = rc
			if r.transport != nil {
				_ = r.transport.BroadcastConsensusRoundChange(rc)
			}
		}
	}
	return r.enterRoundLocked(nextRound)
}

func (r *ConsensusReactor) HandleTimeout() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return fmt.Errorf("consensus reactor not running")
	}
	switch r.step {
	case consensus.StepProposal:
		r.step = consensus.StepPrevote
		r.deadline = time.Now().Add(r.cfg.TimeoutFor(consensus.StepPrevote, r.round))
		return r.signAndBroadcastPrevoteLocked(consensus.NilBlockHash)
	case consensus.StepPrevote, consensus.StepPrecommit:
		return r.requestRoundChangeLocked(r.round + 1)
	default:
		return r.requestRoundChangeLocked(r.round + 1)
	}
}

func (r *ConsensusReactor) recordEvidenceLocked(ev consensus.BFTDoubleSignEvidence) {
	id, err := ev.ID()
	if err != nil {
		return
	}
	if _, exists := r.evidence[strings.ToLower(id)]; exists {
		return
	}
	r.evidence[strings.ToLower(id)] = ev
	if r.transport != nil {
		_ = r.transport.BroadcastConsensusEvidence(ev)
	}
}

func (r *ConsensusReactor) HandleEvidence(ev consensus.BFTDoubleSignEvidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ev.ValidateBasic(); err != nil {
		return err
	}
	var validatorID string
	var height uint64
	switch ev.Step {
	case consensus.StepPrevote:
		validatorID, height = ev.Prevote.VoteA.ValidatorID, ev.Prevote.VoteA.Height
	case consensus.StepPrecommit:
		validatorID, height = ev.Precommit.VoteA.ValidatorID, ev.Precommit.VoteA.Height
	}
	if height != r.height {
		return fmt.Errorf("evidence height %d not current height %d", height, r.height)
	}
	set := r.bc.CurrentValidatorSet(height)
	v, _, ok := set.Find(validatorID)
	if !ok {
		return fmt.Errorf("evidence validator not active")
	}
	switch ev.Step {
	case consensus.StepPrevote:
		if err := consensus.VerifyPrevote(ev.Prevote.VoteA, v); err != nil {
			return err
		}
		if err := consensus.VerifyPrevote(ev.Prevote.VoteB, v); err != nil {
			return err
		}
	case consensus.StepPrecommit:
		if err := consensus.VerifyVote(ev.Precommit.VoteA, v); err != nil {
			return err
		}
		if err := consensus.VerifyVote(ev.Precommit.VoteB, v); err != nil {
			return err
		}
	}
	id, _ := ev.ID()
	r.evidence[strings.ToLower(id)] = ev
	return nil
}

func (r *ConsensusReactor) HandleFinalizedBlock(b *Block) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b == nil {
		return fmt.Errorf("nil finalized block")
	}
	head := r.bc.HeadSnapshot()
	if head == nil {
		return fmt.Errorf("chain head unavailable")
	}
	if b.Index <= head.Index {
		if b.Index == head.Index && strings.EqualFold(b.Hash, head.Hash) {
			// The same finalized block may already have won the race through the
			// ordinary chain-sync protocol. Reconcile the pacemaker even though no
			// blockchain append is needed here.
			next := uint64(head.Index + 1)
			if r.running && next > r.height {
				return r.startHeightLocked(next)
			}
			return nil
		}
		return fmt.Errorf("stale/conflicting finalized block height %d", b.Index)
	}
	if b.Index != head.Index+1 {
		return fmt.Errorf("finalized block gap: got %d want %d", b.Index, head.Index+1)
	}
	if err := r.bc.AppendBlock(cloneBlockForConsensus(b)); err != nil {
		return err
	}
	if r.running {
		newHead := r.bc.HeadSnapshot()
		if newHead == nil {
			return fmt.Errorf("chain head unavailable after finalized append")
		}
		return r.startHeightLocked(uint64(newHead.Index + 1))
	}
	return nil
}
