package blockchain

import (
	"fmt"
	"strings"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

// BuildConsensusProposal creates and fully executes the next block proposal but
// does not persist or commit it. The scheduled validator must collect a quorum
// certificate before FinalizeConsensusProposal can make the block final.
// stageConsensusMempool refreshes proposer input immediately before a new
// consensus proposal is built. Consensus mode does not use the legacy mining
// loop, so reading only PendingTXsV2 would leave admitted transactions in the
// mempool while validators continued finalizing empty blocks.
//
// Snapshots are non-destructive. Transactions are removed only after their
// finalized block has been persisted and accepted.
func (bc *Blockchain) stageConsensusMempool() {
	if bc.mempool == nil {
		return
	}

	v1 := bc.mempool.SnapshotTransactions()
	v2 := bc.mempool.SnapshotTransactionsV2()

	// Preserve explicitly staged fixture input when the mempool is empty.
	if len(v1) == 0 && len(v2) == 0 {
		return
	}

	bc.SetPendingTransactions(v1, v2)
}

func (bc *Blockchain) BuildConsensusProposal(proposerID string, round uint64) (*Block, error) {
	bc.stageConsensusMempool()

	bc.mu.RLock()
	defer bc.mu.RUnlock()

	latest := bc.Blocks[len(bc.Blocks)-1]
	nextHeight := latest.Index + 1
	if !bc.v2Config.ConsensusEnabledAt(nextHeight) {
		return nil, fmt.Errorf("validator consensus is not active at height %d", nextHeight)
	}

	preview := bc.validators.Clone()
	preview.AdvanceHeight(uint64(nextHeight))
	set := preview.ActiveSet(uint64(nextHeight))
	expected, ok := set.Proposer(uint64(nextHeight), round)
	if !ok {
		return nil, fmt.Errorf("no active validators for height %d", nextHeight)
	}
	if !strings.EqualFold(expected.ID, proposerID) {
		return nil, fmt.Errorf("validator %s is not scheduled proposer; expected %s", proposerID, expected.ID)
	}

	rewardTX := NewSystemReward(expected.RewardAddress, bc.MiningReward)
	txs := append([]*Transaction{rewardTX}, bc.PendingTXs...)
	b := NewBlockV2(txs, bc.PendingTXsV2, latest.Hash, nextHeight, poHWithTicks(latest.ProofOfTime, bc.v2Config.EffectivePoHTicks()), bc.v2Config.ChainID)
	b.AuthorValidatorID = expected.ID
	b.ProposerID = expected.ID
	b.ConsensusRound = round
	b.ValidRound = -1
	b.ValidPrevoteCertificate = nil
	prepared, err := bc.prepareV2Commitments(b)
	if err != nil {
		return nil, fmt.Errorf("prepare consensus proposal: %w", err)
	}
	if !strings.EqualFold(prepared.validatorSet.Root(), set.Root()) {
		return nil, fmt.Errorf("validator set changed while building proposal")
	}
	// Consensus-active blocks use a canonical zero PoW nonce. Their authority is
	// established by the scheduled proposer and quorum certificate; retaining the
	// legacy brute-force loop here would make BFT liveness depend on random mining
	// latency and could cause honest proposers to time out while building a block.
	b.ProofOfWork = 0
	b.Hash = b.GenerateHash()
	if err := ValidateBlockAgainstConfig(latest, b, bc.MiningReward, bc.v2Config); err != nil {
		return nil, fmt.Errorf("proposal structural validation: %w", err)
	}
	if err := bc.validateConsensusProposal(b, prepared.validatorSet, false); err != nil {
		return nil, err
	}
	return cloneBlockForConsensus(b), nil
}

// ReproposeConsensusValue reuses a block that already obtained a 2/3 prevote
// certificate in an earlier round. Current proposer/round metadata may change,
// while AuthorValidatorID, reward transaction, execution payload and block hash
// remain unchanged. This is the Phase 3D safe-value path for view changes.
func (bc *Blockchain) ReproposeConsensusValue(proposerID string, round uint64, valid *Block, validRound uint64, validQC consensus.PrevoteCertificate) (*Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if valid == nil {
		return nil, fmt.Errorf("nil valid value")
	}
	latest := bc.Blocks[len(bc.Blocks)-1]
	nextHeight := latest.Index + 1
	if valid.Index != nextHeight || !bc.v2Config.ConsensusEnabledAt(nextHeight) {
		return nil, fmt.Errorf("valid value height %d is not current consensus height %d", valid.Index, nextHeight)
	}
	if validRound >= round {
		return nil, fmt.Errorf("valid round %d must be older than proposal round %d", validRound, round)
	}
	preview := bc.validators.Clone()
	preview.AdvanceHeight(uint64(nextHeight))
	set := preview.ActiveSet(uint64(nextHeight))
	expected, ok := set.Proposer(uint64(nextHeight), round)
	if !ok || !strings.EqualFold(expected.ID, proposerID) {
		return nil, fmt.Errorf("validator %s is not scheduled proposer for round %d", proposerID, round)
	}
	if err := validQC.Verify(set); err != nil {
		return nil, fmt.Errorf("valid-value prevote certificate: %w", err)
	}
	if validQC.Round != validRound || !strings.EqualFold(validQC.BlockHash, valid.Hash) {
		return nil, fmt.Errorf("valid-value certificate does not certify block/round")
	}
	priorAuthor, ok := set.Proposer(uint64(nextHeight), validRound)
	if !ok || !strings.EqualFold(priorAuthor.ID, valid.AuthorValidatorID) {
		return nil, fmt.Errorf("valid block author is not proposer of certified valid round")
	}
	b := cloneBlockForConsensus(valid)
	originalHash := b.Hash
	b.ProposerID = expected.ID
	b.ConsensusRound = round
	b.ValidRound = int64(validRound)
	b.ValidPrevoteCertificate = clonePrevoteQC(&validQC)
	b.FinalityCertificate = nil
	if got := b.GenerateHash(); !strings.EqualFold(got, originalHash) {
		return nil, fmt.Errorf("reproposal changed block content hash")
	}
	if err := ValidateBlockAgainstConfig(latest, b, bc.MiningReward, bc.v2Config); err != nil {
		return nil, fmt.Errorf("reproposal structural validation: %w", err)
	}
	ex, err := bc.validateV2Commitments(b)
	if err != nil {
		return nil, err
	}
	if err := bc.validateConsensusProposal(b, ex.validatorSet, false); err != nil {
		return nil, err
	}
	return b, nil
}

// ValidateConsensusProposalForVote re-executes all state transitions and checks
// proposer scheduling before a validator signs anything.
func (bc *Blockchain) ValidateConsensusProposalForVote(b *Block) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if b == nil {
		return fmt.Errorf("nil proposal")
	}
	if !bc.v2Config.ConsensusEnabledAt(b.Index) {
		return fmt.Errorf("consensus not active at proposal height")
	}
	latest := bc.Blocks[len(bc.Blocks)-1]
	if err := ValidateBlockAgainstConfig(latest, b, bc.MiningReward, bc.v2Config); err != nil {
		return err
	}
	ex, err := bc.validateV2Commitments(b)
	if err != nil {
		return err
	}
	return bc.validateConsensusProposal(b, ex.validatorSet, false)
}

func (bc *Blockchain) SignConsensusVote(b *Block, validatorID string, signer *wallet.Wallet) (consensus.Vote, error) {
	if signer == nil {
		return consensus.Vote{}, fmt.Errorf("nil validator signer")
	}
	if err := bc.ValidateConsensusProposalForVote(b); err != nil {
		return consensus.Vote{}, err
	}

	bc.mu.RLock()
	defer bc.mu.RUnlock()
	preview := bc.validators.Clone()
	preview.AdvanceHeight(uint64(b.Index))
	set := preview.ActiveSet(uint64(b.Index))
	v, _, ok := set.Find(validatorID)
	if !ok {
		return consensus.Vote{}, fmt.Errorf("validator %s is not active", validatorID)
	}
	if !wallet.AddressEqual(signer.Address(), v.ConsensusAddress) {
		return consensus.Vote{}, fmt.Errorf("signer %s is not validator consensus key %s", signer.Address(), v.ConsensusAddress)
	}
	vote, err := consensus.NewSignedVote(bc.v2Config.ChainID, uint64(b.Index), b.ConsensusRound, b.Hash, b.ValidatorSetRoot, v.ID, signer)
	if err != nil {
		return consensus.Vote{}, err
	}
	// Local safety rule: a consensus key may certify at most one proposal hash
	// per chain/height, across every round. The journal is fsync'd before the
	// vote is returned to networking code, so a restart cannot silently cause
	// an honest validator to double-sign a conflicting height.
	if bc.voteJournalErr != nil {
		return consensus.Vote{}, fmt.Errorf("validator vote journal unavailable: %w", bc.voteJournalErr)
	}
	if bc.voteJournal == nil {
		return consensus.Vote{}, fmt.Errorf("validator vote journal unavailable")
	}
	if err := bc.voteJournal.Record(vote); err != nil {
		return consensus.Vote{}, err
	}
	return vote, nil
}

func (bc *Blockchain) FinalizeConsensusProposal(b *Block, qc consensus.QuorumCertificate) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if b == nil {
		return fmt.Errorf("nil proposal")
	}
	if !bc.v2Config.ConsensusEnabledAt(b.Index) {
		return fmt.Errorf("consensus not active at height %d", b.Index)
	}
	b = cloneBlockForConsensus(b)
	b.FinalityCertificate = cloneQC(&qc)
	return bc.appendConsensusBlockLocked(b)
}

func (bc *Blockchain) appendConsensusBlockLocked(b *Block) error {
	latest := bc.Blocks[len(bc.Blocks)-1]
	if err := ValidateBlockAgainstConfig(latest, b, bc.MiningReward, bc.v2Config); err != nil {
		return err
	}
	ex, err := bc.validateV2Commitments(b)
	if err != nil {
		return fmt.Errorf("block #%d V2: %w", b.Index, err)
	}
	if err := bc.validateConsensusProposal(b, ex.validatorSet, true); err != nil {
		return err
	}
	if err := bc.storage.SaveBlock(b); err != nil {
		return fmt.Errorf("persist finalized block #%d: %w", b.Index, err)
	}
	bc.Blocks = append(bc.Blocks, b)
	bc.commitV2Execution(ex)
	bc.removeMinedFromMempool(b)
	bc.PendingTXs = nil
	bc.PendingTXsV2 = nil
	if bc.syncer != nil {
		bc.syncer.BroadcastNewBlock(b)
	}
	return nil
}

func (bc *Blockchain) validateConsensusProposal(b *Block, set consensus.ValidatorSet, requireQC bool) error {
	if b == nil {
		return fmt.Errorf("nil consensus block")
	}
	if !bc.v2Config.ConsensusEnabledAt(b.Index) {
		if b.ProposerID != "" || b.FinalityCertificate != nil {
			return fmt.Errorf("consensus metadata before activation")
		}
		return nil
	}
	if len(set.Validators) == 0 || set.TotalPower() == 0 {
		return fmt.Errorf("no active validator quorum at height %d", b.Index)
	}
	if !strings.EqualFold(set.Root(), b.ValidatorSetRoot) {
		return fmt.Errorf("validator set root mismatch")
	}
	expected, ok := set.Proposer(uint64(b.Index), b.ConsensusRound)
	if !ok || !strings.EqualFold(expected.ID, b.ProposerID) {
		return fmt.Errorf("invalid proposer %s for height=%d round=%d", b.ProposerID, b.Index, b.ConsensusRound)
	}
	author, _, ok := set.Find(b.AuthorValidatorID)
	if !ok {
		return fmt.Errorf("block author %s is not active at height %d", b.AuthorValidatorID, b.Index)
	}
	if b.ValidRound < 0 {
		if b.ValidPrevoteCertificate != nil {
			return fmt.Errorf("initial-round proposal cannot carry valid prevote certificate")
		}
		if !strings.EqualFold(author.ID, expected.ID) {
			return fmt.Errorf("uncertified proposal author %s must equal current proposer %s", author.ID, expected.ID)
		}
	} else {
		if uint64(b.ValidRound) >= b.ConsensusRound {
			return fmt.Errorf("valid round %d must precede consensus round %d", b.ValidRound, b.ConsensusRound)
		}
		if b.ValidPrevoteCertificate == nil {
			return fmt.Errorf("reproposal requires valid prevote certificate")
		}
		qc := clonePrevoteQC(b.ValidPrevoteCertificate)
		if err := qc.Verify(set); err != nil {
			return fmt.Errorf("reproposal prevote certificate: %w", err)
		}
		if qc.Round != uint64(b.ValidRound) || !strings.EqualFold(qc.BlockHash, b.Hash) {
			return fmt.Errorf("reproposal certificate does not certify block/valid round")
		}
		priorProposer, ok := set.Proposer(uint64(b.Index), uint64(b.ValidRound))
		if !ok || !strings.EqualFold(priorProposer.ID, author.ID) {
			return fmt.Errorf("block author %s is not proposer of valid round %d", author.ID, b.ValidRound)
		}
	}
	if err := validateRewardRecipient(b, author.RewardAddress); err != nil {
		return err
	}
	if !requireQC {
		if b.FinalityCertificate != nil {
			// A proposal may already carry a QC when being re-validated locally. If
			// present, verify it rather than ignoring authenticated metadata.
			return bc.validateQuorumCertificate(b, set)
		}
		return nil
	}
	return bc.validateQuorumCertificate(b, set)
}

func (bc *Blockchain) validateQuorumCertificate(b *Block, set consensus.ValidatorSet) error {
	qc := b.FinalityCertificate
	if qc == nil {
		return fmt.Errorf("finalized consensus block requires quorum certificate")
	}
	if qc.ChainID != bc.v2Config.ChainID || qc.Height != uint64(b.Index) || qc.Round != b.ConsensusRound ||
		!strings.EqualFold(qc.BlockHash, b.Hash) || !strings.EqualFold(qc.ValidatorSetRoot, b.ValidatorSetRoot) {
		return fmt.Errorf("quorum certificate does not certify this block")
	}
	copyQC := *qc
	copyQC.Votes = append([]consensus.Vote(nil), qc.Votes...)
	if err := copyQC.Verify(set); err != nil {
		return fmt.Errorf("invalid quorum certificate: %w", err)
	}
	return nil
}

func validateRewardRecipient(b *Block, expectedReward string) error {
	for _, tx := range b.Transactions {
		if tx != nil && tx.IsSystem() {
			if !wallet.AddressEqual(tx.Receiver, expectedReward) {
				return fmt.Errorf("system reward receiver %s does not match proposer reward address %s", tx.Receiver, expectedReward)
			}
			return nil
		}
	}
	return fmt.Errorf("system reward transaction missing")
}

func cloneBlockForConsensus(b *Block) *Block {
	if b == nil {
		return nil
	}
	out := *b
	out.Transactions = append([]*Transaction(nil), b.Transactions...)
	out.TransactionsV2 = make([]*protocolv2.TransactionV2, 0, len(b.TransactionsV2))
	for _, tx := range b.TransactionsV2 {
		out.TransactionsV2 = append(out.TransactionsV2, tx.Clone())
	}
	out.ValidPrevoteCertificate = clonePrevoteQC(b.ValidPrevoteCertificate)
	out.FinalityCertificate = cloneQC(b.FinalityCertificate)
	return &out
}

func (bc *Blockchain) CurrentValidatorSet(height uint64) consensus.ValidatorSet {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	preview := bc.validators.Clone()
	preview.AdvanceHeight(height)
	return preview.ActiveSet(height)
}

func (bc *Blockchain) FinalizedHeight() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	for i := len(bc.Blocks) - 1; i >= 0; i-- {
		b := bc.Blocks[i]
		if bc.v2Config.ConsensusEnabledAt(b.Index) {
			if b.FinalityCertificate != nil {
				return b.Index
			}
			continue
		}
		return b.Index
	}
	return 0
}

func (bc *Blockchain) bftSigningValidator(height uint64, validatorID string, signer *wallet.Wallet) (consensus.ValidatorSet, consensus.Validator, error) {
	if signer == nil {
		return consensus.ValidatorSet{}, consensus.Validator{}, fmt.Errorf("nil validator signer")
	}
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if !bc.v2Config.ConsensusEnabledAt(int(height)) {
		return consensus.ValidatorSet{}, consensus.Validator{}, fmt.Errorf("consensus not active at height %d", height)
	}
	preview := bc.validators.Clone()
	preview.AdvanceHeight(height)
	set := preview.ActiveSet(height)
	v, _, ok := set.Find(validatorID)
	if !ok {
		return consensus.ValidatorSet{}, consensus.Validator{}, fmt.Errorf("validator %s is not active", validatorID)
	}
	if !wallet.AddressEqual(signer.Address(), v.ConsensusAddress) {
		return consensus.ValidatorSet{}, consensus.Validator{}, fmt.Errorf("signer %s is not validator consensus key %s", signer.Address(), v.ConsensusAddress)
	}
	return set, v, nil
}

func (bc *Blockchain) recordBFTSignature(rec consensus.BFTSignRecord) error {
	if bc.bftJournalErr != nil {
		return fmt.Errorf("BFT sign journal unavailable: %w", bc.bftJournalErr)
	}
	if bc.bftJournal == nil {
		return fmt.Errorf("BFT sign journal unavailable")
	}
	return bc.bftJournal.Record(rec)
}

func (bc *Blockchain) SignConsensusProposalHeader(b *Block, validatorID string, signer *wallet.Wallet) (consensus.ProposalHeader, error) {
	if b == nil {
		return consensus.ProposalHeader{}, fmt.Errorf("nil proposal")
	}
	if err := bc.ValidateConsensusProposalForVote(b); err != nil {
		return consensus.ProposalHeader{}, err
	}
	set, v, err := bc.bftSigningValidator(uint64(b.Index), validatorID, signer)
	if err != nil {
		return consensus.ProposalHeader{}, err
	}
	expected, ok := set.Proposer(uint64(b.Index), b.ConsensusRound)
	if !ok || !strings.EqualFold(expected.ID, v.ID) || !strings.EqualFold(b.ProposerID, v.ID) {
		return consensus.ProposalHeader{}, fmt.Errorf("validator %s is not proposer for height=%d round=%d", v.ID, b.Index, b.ConsensusRound)
	}
	h, err := consensus.NewSignedProposalHeader(bc.v2Config.ChainID, uint64(b.Index), b.ConsensusRound, b.Hash, b.ValidatorSetRoot, v.ID, b.ValidRound, signer)
	if err != nil {
		return consensus.ProposalHeader{}, err
	}
	if err := bc.recordBFTSignature(consensus.BFTSignRecord{Step: consensus.StepProposal, ChainID: h.ChainID, Height: h.Height, Round: h.Round, ValidatorID: h.ProposerID, BlockHash: h.BlockHash, ValidatorSetRoot: h.ValidatorSetRoot, Signature: h.Signature}); err != nil {
		return consensus.ProposalHeader{}, err
	}
	return h, nil
}

// SignConsensusPrevote signs either a proposal block or nil. Nil prevotes are
// used when the proposer is absent/invalid and allow the pacemaker to change
// views without signing a conflicting block.
func (bc *Blockchain) SignConsensusPrevote(height, round uint64, blockHash, validatorSetRoot, validatorID string, signer *wallet.Wallet) (consensus.Prevote, error) {
	set, v, err := bc.bftSigningValidator(height, validatorID, signer)
	if err != nil {
		return consensus.Prevote{}, err
	}
	if !strings.EqualFold(set.Root(), validatorSetRoot) {
		return consensus.Prevote{}, fmt.Errorf("prevote validator set root mismatch")
	}
	pv, err := consensus.NewSignedPrevote(bc.v2Config.ChainID, height, round, blockHash, validatorSetRoot, v.ID, signer)
	if err != nil {
		return consensus.Prevote{}, err
	}
	if err := bc.recordBFTSignature(consensus.BFTSignRecord{Step: consensus.StepPrevote, ChainID: pv.ChainID, Height: pv.Height, Round: pv.Round, ValidatorID: pv.ValidatorID, BlockHash: pv.BlockHash, ValidatorSetRoot: pv.ValidatorSetRoot, Signature: pv.Signature}); err != nil {
		return consensus.Prevote{}, err
	}
	return pv, nil
}

func (bc *Blockchain) SignConsensusPrecommit(b *Block, validatorID string, signer *wallet.Wallet) (consensus.Vote, error) {
	if b == nil {
		return consensus.Vote{}, fmt.Errorf("nil proposal")
	}
	if err := bc.ValidateConsensusProposalForVote(b); err != nil {
		return consensus.Vote{}, err
	}
	set, v, err := bc.bftSigningValidator(uint64(b.Index), validatorID, signer)
	if err != nil {
		return consensus.Vote{}, err
	}
	if !strings.EqualFold(set.Root(), b.ValidatorSetRoot) {
		return consensus.Vote{}, fmt.Errorf("precommit validator set root mismatch")
	}
	vote, err := consensus.NewSignedVote(bc.v2Config.ChainID, uint64(b.Index), b.ConsensusRound, b.Hash, b.ValidatorSetRoot, v.ID, signer)
	if err != nil {
		return consensus.Vote{}, err
	}
	if err := bc.recordBFTSignature(consensus.BFTSignRecord{Step: consensus.StepPrecommit, ChainID: vote.ChainID, Height: vote.Height, Round: vote.Round, ValidatorID: vote.ValidatorID, BlockHash: vote.BlockHash, ValidatorSetRoot: vote.ValidatorSetRoot, Signature: vote.Signature}); err != nil {
		return consensus.Vote{}, err
	}
	return vote, nil
}

func (bc *Blockchain) SignConsensusRoundChange(height, nextRound uint64, validatorID string, signer *wallet.Wallet) (consensus.RoundChange, error) {
	set, v, err := bc.bftSigningValidator(height, validatorID, signer)
	if err != nil {
		return consensus.RoundChange{}, err
	}
	rc, err := consensus.NewSignedRoundChange(bc.v2Config.ChainID, height, nextRound, set.Root(), v.ID, signer)
	if err != nil {
		return consensus.RoundChange{}, err
	}
	if err := bc.recordBFTSignature(consensus.BFTSignRecord{Step: consensus.StepRoundChange, ChainID: rc.ChainID, Height: rc.Height, Round: rc.NextRound, ValidatorID: rc.ValidatorID, ValidatorSetRoot: rc.ValidatorSetRoot, Signature: rc.Signature}); err != nil {
		return consensus.RoundChange{}, err
	}
	return rc, nil
}
