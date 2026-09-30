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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
		if binary.BigEndian.Uint32(raw[:4]) != size-4 {
			return nil, fmt.Errorf("record size mismatch")
		}
		// Gob matches exported field names across compatible structs; Timestamp is
		// the sole on-disk rename and is decoded separately from TimestampNs.
		var b blockchain.Block
		var stamp struct{ TimestampNs int64 }
		if e = gob.NewDecoder(bytes.NewReader(raw[4:])).Decode(&b); e != nil {
			return nil, e
		}
		if e = gob.NewDecoder(bytes.NewReader(raw[4:])).Decode(&stamp); e != nil {
			return nil, e
		}
		b.Timestamp = time.Unix(0, stamp.TimestampNs)
		if b.Index < 0 || uint64(b.Index) != height || b.GenerateHash() != b.Hash {
			return nil, fmt.Errorf("block identity/hash mismatch")
		}
		return &b, nil
	}
}
func audit(b *blockchain.Block) (map[string]any, error) {
	p := b.APoW
	if b.ProtocolChainID != 4735490 || p == nil || p.ChainID != 4735490 || p.Height != uint64(b.Index) || p.ParentHash != b.PrevHash || p.PoH != b.ProofOfTime {
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
	return map[string]any{"height": b.Index, "hash": b.Hash, "parent_hash": b.PrevHash, "proof": p, "reward_hbt": 50, "reward_tx": rewardID, "certificate": b.FinalityCertificate, "signature_and_work_verified": true}, nil
}
func main() {
	dir := flag.String("data-dir", "", "existing testnet data directory")
	height := flag.Uint64("height", 0, "finalized height to read")
	flag.Parse()
	b, e := readBlock(*dir, *height)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	result, e := audit(b)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if e = json.NewEncoder(os.Stdout).Encode(result); e != nil {
		os.Exit(1)
	}
}
