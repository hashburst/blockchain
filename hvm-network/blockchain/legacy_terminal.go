package blockchain

// This deliberately separate verifier does not relax the live consensus rules.
// A terminal legacy archive is an owner-authorized migration artifact, not a
// block to gossip to unmodified chain-1337 nodes.
import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"hashburst/wallet"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const LegacyCloseOwner = "0x534746AC40019Ec4E19eb3D12d5F716D4f8b7e3e"
const LegacyCloseRecipient = "0xd1Da8D04D767685e53440DbC56803aF350A65333"
const LegacyCloseAnchor = "0000ca5a560580d0a882a1e46808331e414d073dbc2b0125f5310a6a6a81f586"
const LegacyCloseUnits int64 = 45000000000
const LegacyCloseData = "HASHBURST-LEGACY-TERMINAL-v1|source=1337|height=9|hash=" + LegacyCloseAnchor + "|target=4735489|units=45000000000|issuance=0|terminal=10"

// ReadLegacyPair is strictly read-only, bounds this small legacy chain and
// validates every 20-byte index entry, framing length and trailing byte.
func ReadLegacyPair(dir string, count int) ([]*Block, error) {
	if count != 10 && count != 11 {
		return nil, fmt.Errorf("expected 10 or 11 blocks")
	}
	read := func(name string, limit int64) ([]byte, error) {
		p := filepath.Join(dir, name)
		st, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() || st.Size() > limit {
			return nil, fmt.Errorf("invalid bounded regular ledger file")
		}
		f, e := os.Open(p)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		opened, e := f.Stat()
		if e != nil || !os.SameFile(st, opened) {
			return nil, fmt.Errorf("ledger changed")
		}
		b, e := io.ReadAll(io.LimitReader(f, limit+1))
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("ledger too large")
		}
		return b, e
	}
	idx, e := read("blockchain.idx", 220)
	if e != nil {
		return nil, e
	}
	dat, e := read("blockchain.dat", 16<<20)
	if e != nil {
		return nil, e
	}
	if len(idx) != 20*count {
		return nil, fmt.Errorf("index record count mismatch")
	}
	blocks := make([]*Block, 0, count)
	off := uint64(0)
	for n := 0; n < count; n++ {
		rec := idx[n*20 : (n+1)*20]
		height := binary.BigEndian.Uint64(rec)
		pos := binary.BigEndian.Uint64(rec[8:])
		size := uint64(binary.BigEndian.Uint32(rec[16:]))
		if height != uint64(n) || pos != off || size < 5 || size > 1<<20 || pos > uint64(len(dat)) || size > uint64(len(dat))-pos {
			return nil, fmt.Errorf("invalid index at %d", n)
		}
		frame := dat[pos : pos+size]
		if uint64(binary.BigEndian.Uint32(frame[:4])) != size-4 {
			return nil, fmt.Errorf("frame length mismatch")
		}
		var disk blockOnDisk
		dec := gob.NewDecoder(bytes.NewReader(frame[4:]))
		if e = dec.Decode(&disk); e != nil {
			return nil, e
		}
		var extra blockOnDisk
		if e = dec.Decode(&extra); e != io.EOF {
			return nil, fmt.Errorf("trailing gob payload")
		}
		b := blockFromOnDisk(&disk)
		if b.Index != n {
			return nil, fmt.Errorf("payload height mismatch")
		}
		blocks = append(blocks, b)
		off += size
	}
	if off != uint64(len(dat)) {
		return nil, fmt.Errorf("unindexed bytes")
	}
	return blocks, nil
}

func plainLegacy(b *Block) bool {
	return b != nil && b.EffectiveVersion() == BlockVersionLegacy && b.ProtocolChainID == 0 && len(b.TransactionsV2) == 0 && len(b.EthereumTransactions) == 0 && b.APoW == nil && b.EVMStateRoot == "" && b.EVMReceiptsRoot == "" && b.EVMGasUsed == 0 && b.HBTStateRoot == "" && b.HVMStateRoot == "" && b.ReceiptsRoot == "" && b.ValidatorSetRoot == "" && b.ValidatorStateRoot == "" && b.AuthorValidatorID == "" && b.ProposerID == "" && b.ConsensusRound == 0 && b.ValidRound == 0 && b.ValidPrevoteCertificate == nil && b.FinalityCertificate == nil
}
func VerifyLegacyPrefix(blocks []*Block) (*State, error) {
	return verifyLegacyPrefix(blocks, LegacyCloseAnchor, LegacyCloseOwner)
}
func verifyLegacyPrefix(blocks []*Block, anchor, owner string) (*State, error) {
	if len(blocks) != 10 || !plainLegacy(blocks[0]) || blocks[0].Index != 0 || len(blocks[0].Transactions) != 0 || blocks[0].Hash != NewGenesisBlock().Hash || blocks[0].GenerateHash() != blocks[0].Hash {
		return nil, fmt.Errorf("invalid legacy genesis/prefix")
	}
	s := NewState()
	seen := map[string]bool{}
	for i, b := range blocks {
		if !plainLegacy(b) {
			return nil, fmt.Errorf("nonlegacy envelope")
		}
		if i > 0 {
			if e := ValidateBlockAgainst(blocks[i-1], b, DefaultMiningReward); e != nil {
				return nil, e
			}
		}
		for _, tx := range b.Transactions {
			if tx == nil || math.IsNaN(tx.Amount) || math.IsInf(tx.Amount, 0) || tx.Amount < 0 || tx.Amount > 450 || tx.ID != tx.HashTransaction() || seen[tx.ID] {
				return nil, fmt.Errorf("invalid or repeated transaction")
			}
			if e := tx.Verify(); e != nil {
				return nil, e
			}
			seen[tx.ID] = true
		}
		if e := s.ApplyBlock(b); e != nil {
			return nil, e
		}
	}
	if blocks[9].Hash != anchor || len(s.Snapshot()) != 1 || s.BalanceUnits(owner) != LegacyCloseUnits {
		return nil, fmt.Errorf("unapproved branch or economic state")
	}
	return s, nil
}
func VerifyLegacyTerminal(blocks []*Block) (*State, error) {
	return verifyLegacyTerminal(blocks, LegacyCloseAnchor, LegacyCloseOwner, LegacyCloseRecipient, LegacyCloseData)
}
func verifyLegacyTerminal(blocks []*Block, anchor, owner, recipient, data string) (*State, error) {
	if len(blocks) != 11 {
		return nil, fmt.Errorf("terminal archive must have exactly 11 blocks")
	}
	s, e := verifyLegacyPrefix(blocks[:10], anchor, owner)
	if e != nil {
		return nil, e
	}
	b := blocks[10]
	p := blocks[9]
	if !plainLegacy(b) || b.Index != 10 || b.PrevHash != anchor || !b.Timestamp.After(p.Timestamp) || b.Hash != b.GenerateHash() || !MeetsDifficultyAt(b.Hash, Difficulty) || !ValidatePoH(p.ProofOfTime, b.ProofOfTime) || len(b.Transactions) != 1 {
		return nil, fmt.Errorf("invalid terminal block")
	}
	tx := b.Transactions[0]
	if tx == nil || tx.IsSystem() || tx.Sender != owner || tx.Receiver != recipient || tx.Amount != 450 || tx.Data != data || tx.Nonce < 0 {
		return nil, fmt.Errorf("invalid zero-issuance migration transfer")
	}
	if e = tx.Verify(); e != nil {
		return nil, e
	}
	for _, old := range blocks[:10] {
		for _, t := range old.Transactions {
			if t.ID == tx.ID {
				return nil, fmt.Errorf("duplicate terminal transaction")
			}
		}
	}
	if e = s.ApplyBlock(b); e != nil {
		return nil, e
	}
	if s.BalanceUnits(owner) != 0 || s.BalanceUnits(recipient) != LegacyCloseUnits || len(s.Snapshot()) != 1 {
		return nil, fmt.Errorf("terminal supply not conserved")
	}
	return s, nil
}

// BuildLegacyTerminal requires a newly authorized signature. Custody challenge
// signatures cannot be used as transfer signatures.
func BuildLegacyTerminal(prefix []*Block, w *wallet.Wallet, stamp time.Time) ([]*Block, error) {
	if _, e := VerifyLegacyPrefix(prefix); e != nil {
		return nil, e
	}
	if w == nil || !strings.EqualFold(w.Address(), LegacyCloseOwner) {
		return nil, fmt.Errorf("legacy owner key required")
	}
	if !stamp.After(prefix[9].Timestamp) {
		return nil, fmt.Errorf("terminal timestamp must follow anchor")
	}
	tx := NewDataTransaction(LegacyCloseOwner, LegacyCloseRecipient, 450, LegacyCloseData)
	if e := tx.Sign(w); e != nil {
		return nil, e
	}
	b := NewBlock([]*Transaction{tx}, LegacyCloseAnchor, 10, PoH(prefix[9].ProofOfTime))
	b.Timestamp = stamp.UTC()
	if e := b.MineBlock(); e != nil {
		return nil, e
	}
	out := append(append([]*Block{}, prefix...), b)
	_, e := VerifyLegacyTerminal(out)
	return out, e
}
