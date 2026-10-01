package blockchain

import (
	"encoding/binary"
 "bufio"
 "time"
	"fmt"
	"hashburst/consensus"
	"hashburst/hvm"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// OpenExistingBlockchain never creates genesis or repairs an invalid chain.
// The caller must exclusively lock the directory for the entire node lifetime.
func OpenExistingBlockchain(dir string, cfg ProtocolV2Config, genesis string, checkpointHeight int, checkpointHash string) (*Blockchain, error) {
 return openExistingBlockchain(dir,cfg,genesis,checkpointHeight,checkpointHash,false)
}

// OpenExistingBlockchainWithRecovery permits only authenticated node-local caches.
// Offline audits and configuration migrations use OpenExistingBlockchain instead.
func OpenExistingBlockchainWithRecovery(dir string, cfg ProtocolV2Config, genesis string, checkpointHeight int, checkpointHash string) (*Blockchain,error) {
 return openExistingBlockchain(dir,cfg,genesis,checkpointHeight,checkpointHash,true)
}
func openExistingBlockchain(dir string, cfg ProtocolV2Config, genesis string, checkpointHeight int, checkpointHash string, useCheckpoint bool) (*Blockchain,error) {
	started:=time.Now()
 cfg = cfg.detached()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(dir) || checkpointHeight < 0 {
		return nil, fmt.Errorf("absolute data directory and checkpoint required")
	}
	for _, name := range []string{chainFile, indexFile} {
		st, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular state file: %s", name)
		}
	}
	storage := &ChainStorage{durable: true, dir: dir, datPath: filepath.Join(dir, chainFile), idxPath: filepath.Join(dir, indexFile)}
	// The durable loader validates index/frame agreement before decoding each bounded frame.
	blocks, err := storage.LoadAll()
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 || checkpointHeight >= len(blocks) {
		return nil, fmt.Errorf("state missing checkpoint")
	}
	if !strings.EqualFold(blocks[0].Hash, genesis) || blocks[checkpointHeight].Index != checkpointHeight || !strings.EqualFold(blocks[checkpointHeight].Hash, checkpointHash) {
		return nil, fmt.Errorf("genesis/checkpoint mismatch")
	}
	vote, err := consensus.OpenVoteJournal(filepath.Join(dir, "consensus-votes.jsonl"))
	if err != nil {
		return nil, err
	}
	bft, err := consensus.OpenBFTSignJournal(filepath.Join(dir, "consensus-bft-signatures.jsonl"))
	if err != nil {
		return nil, err
	}
	bc := &Blockchain{Blocks: blocks, MiningReward: DefaultMiningReward, storage: storage, state: NewState(), hvmEngine: hvm.NewEngine(nil, cfg.FeePolicy), validators: consensus.NewRegistry(cfg.Validator), voteJournal: vote, bftJournal: bft, receipts: make(map[string]hvm.Receipt), v2Config: cfg}
	bc.checkpointStartup = useCheckpoint && os.Getenv("HVM_FULL_REPLAY") != "1"
 if bc.checkpointStartup {
  seed, e := bc.loadRecoveryCheckpoint()
  if e == nil { bc.startupSeed = seed; bc.checkpointHeight = seed.height; log.Printf("HVM_CHECKPOINT_RESTORED height=%d suffix=%d", seed.height, len(blocks)-seed.height-1) } else { log.Printf("HVM_CHECKPOINT_FALLBACK reason=%v", e) }
 }
 log.Printf("HVM_CHAIN_VERIFY_BEGIN blocks=%d", len(blocks))
	start := 1
 if bc.startupSeed != nil { start = bc.startupSeed.height+1 }
 if err := bc.verifyChainFrom(start); err != nil {
		return nil, fmt.Errorf("verify existing chain: %w", err)
	}
	log.Printf("HVM_CHAIN_REPLAY_BEGIN blocks=%d", len(blocks))
	if err := bc.rebuildProjections(blocks); err != nil {
		return nil, fmt.Errorf("replay existing chain: %w", err)
	}
	bc.recoveryStatus=RecoveryStatus{Mode:"full",CheckpointHeight:-1,ReplayBlocks:len(blocks),ElapsedMillis:time.Since(started).Milliseconds()}
 if bc.startupSeed!=nil { bc.recoveryStatus.Mode="incremental";bc.recoveryStatus.CheckpointHeight=bc.startupSeed.height;bc.recoveryStatus.ReplayBlocks=len(blocks)-bc.startupSeed.height-1 }
 bc.startupSeed = nil
 bc.checkpointStartup = false
 bc.checkpointEnabled = useCheckpoint && os.Getenv("HVM_FULL_REPLAY") != "1"
 log.Printf("HVM_CHAIN_REPLAY_COMPLETE height=%d", bc.Height())
	return bc, nil
}

// CheckValidatorRestart validates persisted locks/QCs without starting or signing.
func (bc *Blockchain) CheckValidatorRestart(validatorID string) error {
	_, err := bc.readRecovery(validatorID)
	return err
}

// Verify complete index/data agreement before the gob loader allocates frames.
func verifyPersistentIndex(dataPath, indexPath string) error {
	d, e := os.Open(dataPath)
	if e != nil {
		return e
	}
	defer d.Close()
	idx, e := os.Open(indexPath)
	if e != nil {
		return e
	}
	defer idx.Close()
	st, e := d.Stat()
	if e != nil {
		return e
	}
	reader := bufio.NewReaderSize(idx, 64<<10)
 var offset int64
	for n := uint64(0); ; n++ {
		var rec [20]byte
		_, e = io.ReadFull(reader, rec[:])
		if e == io.EOF {
			if offset != st.Size() {
				return fmt.Errorf("unindexed chain bytes")
			}
			return nil
		}
		if e != nil {
			return e
		}
		size := binary.BigEndian.Uint32(rec[16:])
		if binary.BigEndian.Uint64(rec[:8]) != n || binary.BigEndian.Uint64(rec[8:16]) != uint64(offset) || size < 4 || size > 64<<20 || int64(size) > st.Size()-offset {
			return fmt.Errorf("invalid persistent chain index")
		}
		var h [4]byte
		if _, e = d.ReadAt(h[:], offset); e != nil {
			return e
		}
		if binary.BigEndian.Uint32(h[:]) != size-4 {
			return fmt.Errorf("chain index/frame size mismatch")
		}
		offset += int64(size)
	}
}
