package blockchain

import (
	"encoding/json"
	"hashburst/consensus"
	"testing"
)

func TestFinalizedCommitmentRejectsUnavailable(t *testing.T) {
	h := NewRPCHandler(&Blockchain{}, nil, 4735490)
	for _, p := range [][]json.RawMessage{nil, {json.RawMessage("-1")}, {json.RawMessage("1.5")}, {json.RawMessage("0")}, {json.RawMessage("18446744073709551615")}} {
		if _, err := h.getFinalizedCommitment(p); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}
func TestFinalizedCommitmentSnapshot(t *testing.T) {
	bc := &Blockchain{Blocks: []*Block{{Index: 0}, {Index: 1, ProtocolChainID: 4735490, Hash: "fixed", HVMStateRoot: "hvm", FinalityCertificate: &consensus.QuorumCertificate{}}}}
	bc.v2Config.ConsensusActivationHeight = 1
	h := NewRPCHandler(bc, nil, 4735490)
	got, err := h.getFinalizedCommitment([]json.RawMessage{json.RawMessage("1")})
	if err != nil {
		t.Fatal(err)
	}
	proof := got.(FinalizedCommitment)
	if proof.Hash != "fixed" || proof.HVMStateRoot != "hvm" || proof.ChainID != 4735490 {
		t.Fatal(proof)
	}
	if proof.Certificate == bc.Blocks[1].FinalityCertificate {
		t.Fatal("certificate aliases chain")
	}
	bc.Blocks[1].FinalityCertificate = nil
	if _, err := h.getFinalizedCommitment([]json.RawMessage{json.RawMessage("1")}); err == nil {
		t.Fatal("accepted unfinalized")
	}
}
