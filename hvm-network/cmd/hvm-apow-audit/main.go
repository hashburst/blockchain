// hvm-apow-audit reads one committed storage record without opening a runtime,
// taking ownership of a data directory, signing, or writing any node state.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/blockchain"
	"hashburst/ledger"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	mainnetChainID uint64 = 4735489
	testnetChainID uint64 = 4735490
)

func supportedChainID(chainID uint64) bool {
	return chainID == mainnetChainID || chainID == testnetChainID
}

func decodeStorageRecord(raw []byte) (*blockchain.Block, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("storage record too short")
	}
	if string(raw[:4]) == "HBX2" {
		payload, err := ledger.FramePayload(raw)
		if err != nil {
			return nil, err
		}
		b := new(blockchain.Block)
		if err = blockchain.DecodeLedgerBlockInto(payload, b); err != nil {
			return nil, err
		}
		return b, nil
	}
	if binary.BigEndian.Uint32(raw[:4]) != uint32(len(raw)-4) {
		return nil, fmt.Errorf("record size mismatch")
	}
	var b blockchain.Block
	var stamp struct{ TimestampNs int64 }
	if err := gob.NewDecoder(bytes.NewReader(raw[4:])).Decode(&b); err != nil {
		return nil, err
	}
	if err := gob.NewDecoder(bytes.NewReader(raw[4:])).Decode(&stamp); err != nil {
		return nil, err
	}
	b.Timestamp = time.Unix(0, stamp.TimestampNs)
	return &b, nil
}

func readBlock(dir string, height uint64) (*blockchain.Block, error) {
	idx, e := os.Open(filepath.Join(dir, "blockchain.idx"))
	if e != nil {
		return nil, e
	}
	defer idx.Close()
	reader := bufio.NewReaderSize(idx, 65536)
	var entry [20]byte
	for {
		if _, e = io.ReadFull(reader, entry[:]); e != nil {
			return nil, e
		}
		h := binary.BigEndian.Uint64(entry[:8])
		if h != height {
			continue
		}
		off := binary.BigEndian.Uint64(entry[8:16])
		size := binary.BigEndian.Uint32(entry[16:])
		if size < 5 || size > 8<<20 || off > 1<<62 {
			return nil, fmt.Errorf("invalid storage index")
		}
		dat, e := os.Open(filepath.Join(dir, "blockchain.dat"))
		if e != nil {
			return nil, e
		}
		defer dat.Close()
		raw := make([]byte, size)
		if _, e = dat.ReadAt(raw, int64(off)); e != nil {
			return nil, e
		}
		b, e := decodeStorageRecord(raw)
		if e != nil {
			return nil, e
		}
		if b.Index < 0 || uint64(b.Index) != height || b.GenerateHash() != b.Hash {
			return nil, fmt.Errorf("block identity/hash mismatch")
		}
		return b, nil
	}
}

func audit(b *blockchain.Block, expectedChainID uint64) (map[string]any, error) {
	if !supportedChainID(expectedChainID) {
		return nil, fmt.Errorf("unsupported HashBurst chain ID %d", expectedChainID)
	}
	p := b.APoW
	if b.ProtocolChainID != expectedChainID || p == nil || p.ChainID != expectedChainID || p.Height != uint64(b.Index) || p.ParentHash != b.PrevHash || p.PoH != b.ProofOfTime {
		return nil, fmt.Errorf("work/block binding mismatch")
	}
	if e := p.Verify(); e != nil {
		return nil, e
	}
	rewards := 0
	var rewardID string
	for _, tx := range b.Transactions {
		if tx != nil && tx.IsSystem() {
			rewards++
			if tx.Amount != 50 || !strings.EqualFold(tx.Receiver, p.Beneficiary) || tx.ID != tx.HashTransaction() {
				return nil, fmt.Errorf("reward mismatch")
			}
			rewardID = tx.ID
		}
	}
	if rewards != 1 || b.FinalityCertificate == nil {
		return nil, fmt.Errorf("exactly one reward and finality certificate required")
	}
	return map[string]any{
		"chain_id":                    expectedChainID,
		"height":                      b.Index,
		"hash":                        b.Hash,
		"parent_hash":                 b.PrevHash,
		"proof":                       p,
		"beneficiary":                 p.Beneficiary,
		"system_reward_count":         rewards,
		"reward_hbt":                  50,
		"reward_units":                int64(5000000000),
		"reward_tx":                   rewardID,
		"certificate":                 b.FinalityCertificate,
		"signature_and_work_verified": true,
	}, nil
}

func main() {
	dir := flag.String("data-dir", "", "existing HashBurst data directory")
	height := flag.Uint64("height", 0, "finalized height to read")
	chainID := flag.Uint64("chain-id", 0, "expected HashBurst chain ID (4735489 mainnet or 4735490 testnet)")
	flag.Parse()
	if !supportedChainID(*chainID) {
		fmt.Fprintf(os.Stderr, "unsupported HashBurst chain ID %d\n", *chainID)
		os.Exit(1)
	}
	b, e := readBlock(*dir, *height)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	result, e := audit(b, *chainID)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if e = json.NewEncoder(os.Stdout).Encode(result); e != nil {
		os.Exit(1)
	}
}
