package blockchain

import "testing"

func TestPhase3ENodeClassRolesAndTEPEligibility(t *testing.T) {
	base := NodeRecordV2{
		NodeRecord:      NodeRecord{NodeID: "phase3e-hpc", PeerID: "12D3KooWPhase3E", ChainID: 1337, TEPPubkey: "0123456789abcdef"},
		RecordVersion:   NodeRecordVersion2,
		NodeClass:       NodeClassHPC,
		Roles:           []NodeRole{NodeRoleFull, NodeRoleEdge},
		TEPEnabled:      true,
		Capabilities:    []string{NodeCapabilityHVMV2, NodeCapabilityValidatorV2, NodeCapabilityConsensusV2},
		ProtocolVersion: BlockVersionV2,
	}
	if err := base.ValidateForValidator(1337); err != nil {
		t.Fatalf("HPC full+edge TEP node should be validator eligible: %v", err)
	}
	storageOnly := base
	storageOnly.Roles = []NodeRole{NodeRoleStorage}
	if err := storageOnly.ValidateForValidator(1337); err == nil {
		t.Fatal("storage-only role unexpectedly validator eligible")
	}
	noTEP := base
	noTEP.TEPEnabled = false
	if err := noTEP.ValidateForValidator(1337); err == nil {
		t.Fatal("node without TEP unexpectedly validator eligible")
	}
	badClass := base
	badClass.NodeClass = "unknown"
	if err := badClass.ValidateForValidator(1337); err == nil {
		t.Fatal("unknown node class unexpectedly accepted")
	}
}
