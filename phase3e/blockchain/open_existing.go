package blockchain

import (
	"encoding/binary"
	"fmt"
	"hashburst/consensus"
	"hashburst/hvm"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// OpenExistingBlockchain never creates genesis or repairs an invalid chain.
// The caller must exclusively lock the directory for the entire node lifetime.
func OpenExistingBlockchain(dir string, cfg ProtocolV2Config, genesis string, checkpointHeight int, checkpointHash string) (*Blockchain, error) {
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
	if err := verifyPersistentIndex(storage.datPath, storage.idxPath); err != nil {
		return nil, err
	}
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
	if err := bc.VerifyChain(); err != nil {
		return nil, fmt.Errorf("verify existing chain: %w", err)
	}
	if err := bc.rebuildProjections(blocks); err != nil {
		return nil, fmt.Errorf("replay existing chain: %w", err)
	}
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
	var offset int64
	for n := uint64(0); ; n++ {
		var rec [20]byte
		_, e = io.ReadFull(idx, rec[:])
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
