package blockchain

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"hashburst/consensus"
	"hashburst/hvm"
	"hashburst/protocolv2"
)

// Blockchain keeps V1 compatibility while Phase 3B adds dark, activation-gated
// Protocol V2/HVM projections. The chain remains the source of truth; native
// HBT state, HVM state and receipts are deterministic projections of blocks.
type Blockchain struct {
	Blocks       []*Block
	PendingTXs   []*Transaction
	PendingTXsV2 []*protocolv2.TransactionV2
	MiningReward float64

	mu               sync.RWMutex
	state            *State
	hvmEngine        *hvm.Engine
	validators       *consensus.Registry
	voteJournal      *consensus.VoteJournal // legacy Phase 3C guard; Phase 3D uses bftJournal
	voteJournalErr   error
	bftJournal       *consensus.BFTSignJournal
	bftJournalErr    error
	receipts         map[string]hvm.Receipt
	v2Config         ProtocolV2Config
	mempool          *Mempool
	storage          *ChainStorage
	syncer           BlockBroadcaster
	consensusReactor *ConsensusReactor
	consensusNetwork *Libp2pConsensusNetwork
}

type BlockBroadcaster interface {
	BroadcastNewBlock(b *Block)
}

func NewBlockchain() *Blockchain { return NewBlockchainWithDir("") }

func NewBlockchainWithDir(dir string) *Blockchain {
	return NewBlockchainWithDirAndV2Config(dir, DefaultProtocolV2Config())
}

func NewBlockchainWithDirAndV2Config(dir string, cfg ProtocolV2Config) *Blockchain {
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("invalid Protocol V2 config: %v", err))
	}
	storage := NewChainStorage(dir)
	voteJournal, voteJournalErr := consensus.OpenVoteJournal(filepath.Join(storage.dir, "consensus-votes.jsonl"))
	bftJournal, bftJournalErr := consensus.OpenBFTSignJournal(filepath.Join(storage.dir, "consensus-bft-signatures.jsonl"))
	bc := &Blockchain{
		MiningReward:   DefaultMiningReward,
		storage:        storage,
		state:          NewState(),
		hvmEngine:      hvm.NewEngine(nil, cfg.FeePolicy),
		validators:     consensus.NewRegistry(cfg.Validator),
		voteJournal:    voteJournal,
		voteJournalErr: voteJournalErr,
		bftJournal:     bftJournal,
		bftJournalErr:  bftJournalErr,
		receipts:       make(map[string]hvm.Receipt),
		v2Config:       cfg,
	}

	if storage.Exists() {
		blocks, err := storage.LoadAll()
		if err != nil {
			log.Printf("Storage load error: %v — starting fresh", err)
		} else if len(blocks) > 0 {
			bc.Blocks = blocks
			log.Printf("Blockchain loaded from disk: %d blocks (height=%d)", len(blocks), blocks[len(blocks)-1].Index)
			stats := storage.Stats()
			log.Printf("Storage: %.2f MB on disk", stats["size_mb"])

			if err := bc.VerifyChain(); err != nil {
				log.Printf("ATTENZIONE: catena su disco non valida: %v", err)
			} else if err := bc.rebuildProjections(blocks); err != nil {
				log.Printf("ATTENZIONE: proiezioni HBT/HVM non ricostruibili: %v", err)
			} else {
				log.Printf("Chain verified: %d blocks, genesis %s...", len(blocks), blocks[0].Hash[:12])
				log.Printf("Stato ricostruito: %d indirizzi con saldo; HBT root=%s... HVM root=%s...",
					len(bc.state.Snapshot()), shortHash(bc.state.Root()), shortHash(bc.hvmEngine.State().Root()))
			}
			return bc
		}
	}

	log.Println("No blockchain data found — creating genesis block")
	genesis := NewGenesisBlock()
	log.Printf("Genesis (deterministic): %s", genesis.Hash)
	bc.Blocks = []*Block{genesis}
	if err := storage.SaveBlock(genesis); err != nil {
		log.Printf("Warning: could not save genesis block: %v", err)
	}
	return bc
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func (bc *Blockchain) SetMempool(mp *Mempool)       { bc.mempool = mp }
func (bc *Blockchain) SetSyncer(s BlockBroadcaster) { bc.syncer = s }

func (bc *Blockchain) attachConsensusReactor(r *ConsensusReactor) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.consensusReactor = r
}

func (bc *Blockchain) attachConsensusNetwork(n *Libp2pConsensusNetwork) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.consensusNetwork = n
}

func (bc *Blockchain) ConsensusRuntimeStatus() (ConsensusReactorStatus, ConsensusNetworkStatus, bool) {
	bc.mu.RLock()
	r := bc.consensusReactor
	n := bc.consensusNetwork
	bc.mu.RUnlock()
	if r == nil {
		return ConsensusReactorStatus{}, ConsensusNetworkStatus{}, false
	}
	rs := r.Status()
	var ns ConsensusNetworkStatus
	if n != nil {
		ns = n.Status()
	}
	return rs, ns, true
}

func (bc *Blockchain) ConsensusEvidence() []consensus.BFTDoubleSignEvidence {
	bc.mu.RLock()
	r := bc.consensusReactor
	bc.mu.RUnlock()
	if r == nil {
		return nil
	}
	return r.PendingEvidence()
}

// syncConsensusReactorToHead reconciles the pacemaker with a finalized chain
// head that arrived through the ordinary chain-sync protocol. Real Phase 3E
// nodes run chain sync and BFT gossip concurrently, so the finalized block can
// legitimately reach a peer through /hashburst/1.0.0 before the dedicated
// consensus-finalized envelope. The blockchain is already authoritative after
// QC validation; the reactor must therefore advance to head+1 instead of
// remaining stuck on an already-finalized height.
func (bc *Blockchain) syncConsensusReactorToHead() error {
	bc.mu.RLock()
	r := bc.consensusReactor
	var head *Block
	if len(bc.Blocks) > 0 {
		head = cloneBlockForConsensus(bc.Blocks[len(bc.Blocks)-1])
	}
	bc.mu.RUnlock()
	if r == nil || head == nil {
		return nil
	}
	return r.SyncToFinalizedHead(head)
}

// SetPendingTransactions stages proposer input without removing it from the
// mempool. Entries are removed only after the resulting block is accepted.
func (bc *Blockchain) SetPendingTransactions(v1 []*Transaction, v2 []*protocolv2.TransactionV2) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.PendingTXs = append([]*Transaction(nil), v1...)
	bc.PendingTXsV2 = make([]*protocolv2.TransactionV2, 0, len(v2))
	for _, tx := range v2 {
		bc.PendingTXsV2 = append(bc.PendingTXsV2, tx.Clone())
	}
}

func (bc *Blockchain) AddBlock(minerAddress string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	latest := bc.Blocks[len(bc.Blocks)-1]
	nextHeight := latest.Index + 1
	if bc.v2Config.ConsensusEnabledAt(nextHeight) {
		return fmt.Errorf("validator consensus active at height %d: use BuildConsensusProposal/FinalizeConsensusProposal", nextHeight)
	}
	rewardTX := NewSystemReward(minerAddress, bc.MiningReward)
	txs := append([]*Transaction{rewardTX}, bc.PendingTXs...)

	var newBlock *Block
	var prepared *blockExecutionV2
	var err error
	if bc.v2Config.EnabledAt(nextHeight) {
		newBlock = NewBlockV2(txs, bc.PendingTXsV2, latest.Hash, nextHeight, poHWithTicks(latest.ProofOfTime, bc.v2Config.EffectivePoHTicks()), bc.v2Config.ChainID)
		prepared, err = bc.prepareV2Commitments(newBlock)
		if err != nil {
			return fmt.Errorf("preparazione Protocol V2: %w", err)
		}
	} else {
		if len(bc.PendingTXsV2) != 0 {
			return fmt.Errorf("Protocol V2 non attivo all'altezza %d", nextHeight)
		}
		newBlock = NewBlock(txs, latest.Hash, nextHeight, poHWithTicks(latest.ProofOfTime, bc.v2Config.EffectivePoHTicks()))
	}

	if err := newBlock.MineBlockWithDifficulty(bc.v2Config.LegacyDifficulty()); err != nil {
		return fmt.Errorf("mining fallito: %w", err)
	}
	if err := ValidateBlockAgainstConfig(latest, newBlock, bc.MiningReward, bc.v2Config); err != nil {
		return fmt.Errorf("blocco appena minato non valido: %w", err)
	}

	if newBlock.EffectiveVersion() < BlockVersionV2 {
		if err := bc.state.CanApplyBlock(newBlock); err != nil {
			return fmt.Errorf("blocco non applicabile allo stato: %w", err)
		}
	}

	// Persistence is part of successful proposal. If it fails, neither state nor
	// mempool is mutated, so the same transactions may be retried safely.
	if err := bc.storage.SaveBlock(newBlock); err != nil {
		return fmt.Errorf("persist block #%d: %w", newBlock.Index, err)
	}

	bc.Blocks = append(bc.Blocks, newBlock)
	if newBlock.EffectiveVersion() >= BlockVersionV2 {
		bc.commitV2Execution(prepared)
	} else {
		_ = bc.state.ApplyBlock(newBlock)
	}
	bc.removeMinedFromMempool(newBlock)
	bc.PendingTXs = nil
	bc.PendingTXsV2 = nil

	log.Printf("Block #%d v%d | hash: %s... | txs v1=%d v2=%d | PoW: %d | PoH: %d",
		newBlock.Index, newBlock.EffectiveVersion(), newBlock.Hash[:12],
		len(newBlock.Transactions), len(newBlock.TransactionsV2), newBlock.ProofOfWork, newBlock.ProofOfTime)

	if bc.syncer != nil {
		bc.syncer.BroadcastNewBlock(newBlock)
	}
	return nil
}

func (bc *Blockchain) AppendBlock(b *Block) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if b != nil && bc.v2Config.ConsensusEnabledAt(b.Index) {
		return bc.appendConsensusBlockLocked(cloneBlockForConsensus(b))
	}

	latest := bc.Blocks[len(bc.Blocks)-1]
	if err := ValidateBlockAgainstConfig(latest, b, bc.MiningReward, bc.v2Config); err != nil {
		return err
	}

	var executed *blockExecutionV2
	var err error
	if b.EffectiveVersion() >= BlockVersionV2 {
		executed, err = bc.validateV2Commitments(b)
		if err != nil {
			return fmt.Errorf("blocco #%d Protocol V2: %w", b.Index, err)
		}
	} else if err := bc.state.CanApplyBlock(b); err != nil {
		return fmt.Errorf("blocco #%d, fondi: %w", b.Index, err)
	}

	if err := bc.storage.SaveBlock(b); err != nil {
		return fmt.Errorf("persist block #%d: %w", b.Index, err)
	}
	bc.Blocks = append(bc.Blocks, b)
	if executed != nil {
		bc.commitV2Execution(executed)
	} else {
		_ = bc.state.ApplyBlock(b)
	}
	bc.removeMinedFromMempool(b)
	log.Printf("Block #%d v%d accepted from network | hash: %s...", b.Index, b.EffectiveVersion(), b.Hash[:12])
	return nil
}

func (bc *Blockchain) commitV2Execution(ex *blockExecutionV2) {
	if ex == nil {
		return
	}
	bc.state.ReplaceWith(ex.state)
	bc.hvmEngine.ReplaceStateWith(ex.hvm)
	bc.validators.ReplaceWith(ex.validators)
	for _, r := range ex.receipts {
		bc.receipts[strings.ToLower(strings.TrimPrefix(r.TxID, "0x"))] = r
	}
}

func (bc *Blockchain) ValidateBlock(b *Block) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return ValidateBlockAgainstConfig(bc.Blocks[len(bc.Blocks)-1], b, bc.MiningReward, bc.v2Config)
}

// ValidateBlockAgainst keeps legacy callers source-compatible and therefore
// assumes Protocol V2 disabled. Consensus-aware Blockchain paths use the Config variant.
func ValidateBlockAgainst(prev *Block, b *Block, miningReward float64) error {
	return ValidateBlockAgainstConfig(prev, b, miningReward, DefaultProtocolV2Config())
}

func ValidateBlockAgainstConfig(prev *Block, b *Block, miningReward float64, cfg ProtocolV2Config) error {
	if b == nil {
		return fmt.Errorf("blocco nil")
	}
	if b.Index != prev.Index+1 {
		return fmt.Errorf("indice %d: atteso %d", b.Index, prev.Index+1)
	}
	if b.PrevHash != prev.Hash {
		return fmt.Errorf("prevHash %s non corrisponde alla testa %s", b.PrevHash, prev.Hash)
	}
	if b.Timestamp.UnixNano() < prev.Timestamp.UnixNano() {
		return fmt.Errorf("timestamp precedente al blocco padre")
	}

	v2Enabled := cfg.EnabledAt(b.Index)
	if v2Enabled && b.EffectiveVersion() != BlockVersionV2 {
		return fmt.Errorf("blocco #%d deve usare versione V2 dopo activation height", b.Index)
	}
	if !v2Enabled && b.EffectiveVersion() >= BlockVersionV2 {
		return fmt.Errorf("blocco V2 prima dell'activation height")
	}
	if b.EffectiveVersion() > BlockVersionV2 {
		return fmt.Errorf("versione blocco non supportata %d", b.EffectiveVersion())
	}

	rewards := 0
	for i, tx := range b.Transactions {
		if tx == nil || !tx.IsWellFormed() {
			return fmt.Errorf("transazione V1 %d malformata", i)
		}
		if tx.IsSystem() {
			rewards++
			if AmountToUnits(tx.Amount) != AmountToUnits(miningReward) {
				return fmt.Errorf("reward %v: attesa %v", tx.Amount, miningReward)
			}
		}
	}
	if rewards != 1 {
		return fmt.Errorf("%d transazioni di reward: attesa esattamente 1", rewards)
	}

	if b.EffectiveVersion() >= BlockVersionV2 {
		if b.ProtocolChainID != cfg.ChainID {
			return fmt.Errorf("protocol chain id %d: atteso %d", b.ProtocolChainID, cfg.ChainID)
		}
		for name, root := range map[string]string{
			"hbt_state_root":       b.HBTStateRoot,
			"hvm_state_root":       b.HVMStateRoot,
			"receipts_root":        b.ReceiptsRoot,
			"validator_set_root":   b.ValidatorSetRoot,
			"validator_state_root": b.ValidatorStateRoot,
		} {
			if !isHex32(root) {
				return fmt.Errorf("%s non valido", name)
			}
		}
		for i, tx := range b.TransactionsV2 {
			if tx == nil {
				return fmt.Errorf("transazione V2 %d nil", i)
			}
			if len(tx.Data) > cfg.MaxTxDataBytes {
				return fmt.Errorf("transazione V2 %d data troppo grande", i)
			}
			if err := tx.Verify(cfg.ChainID); err != nil {
				return fmt.Errorf("transazione V2 %d: %w", i, err)
			}
			if tx.IsSystem() {
				return fmt.Errorf("transazione V2 %d: system type riservato", i)
			}
			if !isProtocolV2ExecutableTxType(tx.Type) {
				return fmt.Errorf("transazione V2 %d: type %s non attivo in Protocol V2", i, tx.Type.String())
			}
			maxRequired, err := cfg.FeePolicy.MaxFeeForLimit(tx.ComputeLimit)
			switch tx.Type {
			case protocolv2.TxHBTTransfer:
				maxRequired, err = cfg.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
			case protocolv2.TxValidatorRegister:
				maxRequired, err = cfg.FeePolicy.ComputeFee(computeValidatorRegister)
			case protocolv2.TxValidatorUnbond:
				maxRequired, err = cfg.FeePolicy.ComputeFee(computeValidatorUnbond)
			case protocolv2.TxValidatorWithdraw:
				maxRequired, err = cfg.FeePolicy.ComputeFee(computeValidatorWithdraw)
			case protocolv2.TxValidatorEvidence:
				maxRequired, err = cfg.FeePolicy.ComputeFee(computeValidatorEvidence)
			}
			if err != nil || tx.MaxFeeUnits < maxRequired {
				return fmt.Errorf("transazione V2 %d max fee insufficiente", i)
			}
		}
		if cfg.ConsensusEnabledAt(b.Index) {
			if !isHex32(b.AuthorValidatorID) || !isHex32(b.ProposerID) {
				return fmt.Errorf("consensus author/proposer id non valido")
			}
		} else if b.AuthorValidatorID != "" || b.ProposerID != "" || b.ConsensusRound != 0 || b.ValidRound != -1 || b.ValidPrevoteCertificate != nil || b.FinalityCertificate != nil {
			return fmt.Errorf("consensus metadata present before consensus activation")
		}
	} else {
		if len(b.TransactionsV2) != 0 {
			return fmt.Errorf("transazioni V2 in blocco legacy")
		}
		// These fields are not part of the legacy block hash; requiring them to be
		// empty prevents unauthenticated metadata from being attached to a V1 block.
		if b.ProtocolChainID != 0 || b.HBTStateRoot != "" || b.HVMStateRoot != "" || b.ReceiptsRoot != "" ||
			b.ValidatorSetRoot != "" || b.ValidatorStateRoot != "" || b.AuthorValidatorID != "" || b.ProposerID != "" || b.ConsensusRound != 0 || b.ValidRound != 0 || b.ValidPrevoteCertificate != nil || b.FinalityCertificate != nil {
			return fmt.Errorf("campi Protocol V2/consensus presenti in blocco legacy")
		}
	}

	if got := b.GenerateHash(); got != b.Hash {
		return fmt.Errorf("hash dichiarato %s..., ricalcolato %s...", shortHash(b.Hash), shortHash(got))
	}
	if cfg.ConsensusEnabledAt(b.Index) {
		if b.ProofOfWork != 0 {
			return fmt.Errorf("consensus block must use canonical ProofOfWork=0")
		}
	} else if !MeetsDifficultyAt(b.Hash, cfg.LegacyDifficulty()) {
		return fmt.Errorf("PoW insufficiente: hash %s... difficulty=%d", shortHash(b.Hash), cfg.LegacyDifficulty())
	}
	if !validatePoHWithTicks(prev.ProofOfTime, b.ProofOfTime, cfg.EffectivePoHTicks()) {
		return fmt.Errorf("PoH non deriva dal blocco padre")
	}
	return nil
}

func isHex32(s string) bool {
	s = strings.TrimPrefix(s, "0x")
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func (bc *Blockchain) VerifyChain() error {
	if len(bc.Blocks) == 0 {
		return fmt.Errorf("catena vuota")
	}
	expected := NewGenesisBlock()
	if bc.Blocks[0].Hash != expected.Hash {
		return fmt.Errorf("genesis %s... non corrisponde a %s...", shortHash(bc.Blocks[0].Hash), shortHash(expected.Hash))
	}
	for i := 1; i < len(bc.Blocks); i++ {
		if err := ValidateBlockAgainstConfig(bc.Blocks[i-1], bc.Blocks[i], bc.MiningReward, bc.v2Config); err != nil {
			return fmt.Errorf("blocco #%d: %w", i, err)
		}
	}
	return nil
}

func (bc *Blockchain) rebuildProjections(blocks []*Block) error {
	st, engine, validators, receipts, err := bc.computeProjections(blocks)
	if err != nil {
		return err
	}
	bc.state = st
	bc.hvmEngine = engine
	bc.validators = validators
	bc.receipts = receipts
	return nil
}

func (bc *Blockchain) TotalPoHTicks() int64 {
	return int64(len(bc.Blocks)-1) * int64(bc.v2Config.EffectivePoHTicks())
}
func (bc *Blockchain) Height() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.Blocks[len(bc.Blocks)-1].Index
}

// HeadSnapshot returns an immutable copy of the current chain head. Consensus
// networking must never read bc.Blocks directly because the reactor has its own
// mutex and can otherwise race with block commit/sync.
func (bc *Blockchain) HeadSnapshot() *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if len(bc.Blocks) == 0 {
		return nil
	}
	return cloneBlockForConsensus(bc.Blocks[len(bc.Blocks)-1])
}

func (bc *Blockchain) Balance(addr string) float64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.state.Balance(addr)
}

func (bc *Blockchain) BalanceUnits(addr string) int64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.state.BalanceUnits(addr)
}

func (bc *Blockchain) Sequence(addr string) uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.state.Sequence(addr)
}

func (bc *Blockchain) HBTStateRoot() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.state.Root()
}

func (bc *Blockchain) HVMStateRoot() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.hvmEngine.State().Root()
}

func (bc *Blockchain) Receipt(txID string) (hvm.Receipt, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	r, ok := bc.receipts[strings.ToLower(strings.TrimPrefix(txID, "0x"))]
	return r, ok
}

func (bc *Blockchain) ProtocolV2Config() ProtocolV2Config     { return bc.v2Config }
func (bc *Blockchain) State() *State                          { return bc.state }
func (bc *Blockchain) HVMEngine() *hvm.Engine                 { return bc.hvmEngine }
func (bc *Blockchain) ValidatorRegistry() *consensus.Registry { return bc.validators }

func (bc *Blockchain) StorageStats() map[string]interface{} { return bc.storage.Stats() }
func (bc *Blockchain) StorageDir() string                   { return bc.storage.dir }

func (bc *Blockchain) ResetStorage() error {
	log.Println("WARNING: resetting blockchain storage")
	_ = os.Remove(bc.storage.datPath)
	_ = os.Remove(bc.storage.idxPath)
	return nil
}
