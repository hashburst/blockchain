package blockchain

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hashburst/protocolv2"
)

// These local types intentionally mirror the exact pre-Phase-3B gob schema.
// They verify that adding fields to blockOnDisk remains backward compatible.
type legacyBlockOnDiskForTest struct {
	Index        int
	TimestampNs  int64
	Transactions []legacyTxOnDiskForTest
	PrevHash     string
	Hash         string
	ProofOfWork  int64
	ProofOfTime  int64
}

type legacyTxOnDiskForTest struct {
	ID        string
	Sender    string
	Receiver  string
	Amount    float64
	Data      string
	Nonce     int64
	PubKey    string
	Signature string
}

func TestStorageLoadsLegacyGobAsV1(t *testing.T) {
	dir := t.TempDir()
	legacy := legacyBlockOnDiskForTest{
		Index:       7,
		TimestampNs: time.Unix(1787000000, 123).UTC().UnixNano(),
		Transactions: []legacyTxOnDiskForTest{{
			ID: "legacy-id", Sender: "sender", Receiver: "receiver", Amount: 1.25,
			Data: "legacy-data", Nonce: 42, PubKey: "pub", Signature: "sig",
		}},
		PrevHash: "prev", Hash: "hash", ProofOfWork: 9, ProofOfTime: 10,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	payload := buf.Bytes()
	var framed bytes.Buffer
	if err := binary.Write(&framed, binary.BigEndian, uint32(len(payload))); err != nil {
		t.Fatal(err)
	}
	framed.Write(payload)
	if err := os.WriteFile(filepath.Join(dir, chainFile), framed.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	storage := NewChainStorage(dir)
	blocks, err := storage.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll legacy gob: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("loaded %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if b.Version != 0 || b.EffectiveVersion() != BlockVersionLegacy {
		t.Fatalf("legacy version=%d effective=%d", b.Version, b.EffectiveVersion())
	}
	if len(b.TransactionsV2) != 0 || b.HBTStateRoot != "" || b.HVMStateRoot != "" || b.ReceiptsRoot != "" {
		t.Fatalf("legacy block unexpectedly acquired V2 fields: %+v", b)
	}
	if len(b.Transactions) != 1 || b.Transactions[0].ID != "legacy-id" || b.Transactions[0].Nonce != 42 {
		t.Fatalf("legacy transaction mismatch: %+v", b.Transactions)
	}
}

func TestStorageV2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	storage := NewChainStorage(dir)
	tx := &protocolv2.TransactionV2{
		Version: protocolv2.Version2, ChainID: 1337, Type: protocolv2.TxContractCall,
		Sender: "0x1111111111111111111111111111111111111111", To: "0x2222222222222222222222222222222222222222",
		ValueUnits: 123, Sequence: 8, ComputeLimit: 400000, MaxFeeUnits: 999,
		Data: []byte(`{"method":"getPayout","args":{}}`), Signature: "abcdef", ID: "v2-id",
	}
	b := &Block{
		Version: BlockVersionV2, ProtocolChainID: 1337, Index: 3, Timestamp: time.Unix(1787001234, 987654321).UTC(),
		Transactions:   []*Transaction{{ID: "v1", Sender: SystemSender, Receiver: "0x3333333333333333333333333333333333333333", Amount: 50, Nonce: 1}},
		TransactionsV2: []*protocolv2.TransactionV2{tx}, PrevHash: "prev", Hash: "hash", ProofOfWork: 11, ProofOfTime: 12,
		HBTStateRoot: p3bHex32("hbt-root"), HVMStateRoot: p3bHex32("hvm-root"), ReceiptsRoot: p3bHex32("receipt-root"),
	}
	if err := storage.SaveBlock(b); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}
	blocks, err := storage.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("loaded %d blocks, want 1", len(blocks))
	}
	got := blocks[0]
	if got.Version != b.Version || got.ProtocolChainID != b.ProtocolChainID || !got.Timestamp.Equal(b.Timestamp) || got.HBTStateRoot != b.HBTStateRoot || got.HVMStateRoot != b.HVMStateRoot || got.ReceiptsRoot != b.ReceiptsRoot {
		t.Fatalf("V2 block metadata mismatch:\n got=%+v\nwant=%+v", got, b)
	}
	if len(got.TransactionsV2) != 1 || !reflect.DeepEqual(got.TransactionsV2[0], tx) {
		t.Fatalf("V2 tx mismatch:\n got=%+v\nwant=%+v", got.TransactionsV2, tx)
	}
}
