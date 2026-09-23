package blockchain

import (
	"encoding/json"
	"strings"
	"testing"

	"hashburst/consensus"
	"hashburst/protocolv2"
	"hashburst/wallet"
)

type sandboxValidator struct {
	nodeID    string
	peerID    string
	operator  *wallet.Wallet
	reward    *wallet.Wallet
	consensus *wallet.Wallet
	id        string
}

func phase3CTestConfig() ProtocolV2Config {
	cfg := DefaultProtocolV2Config()
	cfg.ActivationHeight = 5
	cfg.ConsensusActivationHeight = 7
	cfg.LegacyPoWDifficulty = 1
	cfg.PoHTicksPerBlock = 4_000
	cfg.Validator.MinBondUnits = 10 * AmountScale
	cfg.Validator.ActivationDelay = 1
	cfg.Validator.UnbondingBlocks = 3
	cfg.Validator.JailBlocks = 2
	cfg.Validator.DoubleVoteSlashBPS = 500
	return cfg
}

func newSandboxValidator(t *testing.T, i int) sandboxValidator {
	t.Helper()
	op, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	reward, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	cv, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	id, err := consensus.ValidatorID(cv.PublicKeyHexCompressed())
	if err != nil {
		t.Fatal(err)
	}
	return sandboxValidator{nodeID: "sandbox-node-" + string(rune('a'+i)), peerID: "12D3KooWSandboxPeer" + string(rune('A'+i)), operator: op, reward: reward, consensus: cv, id: id}
}

func signedNodeRegistration(t *testing.T, v sandboxValidator, chainID uint64) *Transaction {
	t.Helper()
	tx := NewNodeRegistrationV2(v.operator.Address(), NodeRecordV2{
		NodeRecord:      NodeRecord{NodeID: v.nodeID, PeerID: v.peerID, Version: "phase3d-test", ChainID: int(chainID), TEPPort: 47777, TEPPubkey: "phase3e-test-tep"},
		NodeClass:       NodeClassNode,
		Roles:           []NodeRole{NodeRoleFull},
		TEPEnabled:      true,
		Capabilities:    []string{NodeCapabilityHVMV2, NodeCapabilityValidatorV2, NodeCapabilityConsensusV2},
		ProtocolVersion: BlockVersionV2,
	})
	if err := tx.Sign(v.operator); err != nil {
		t.Fatal(err)
	}
	return tx
}

func validatorRegisterTx(t *testing.T, cfg ProtocolV2Config, v sandboxValidator, sequence uint64) *protocolv2.TransactionV2 {
	t.Helper()
	req := consensus.RegisterRequest{NodeID: v.nodeID, PeerID: v.peerID, ConsensusPubKey: v.consensus.PublicKeyHexCompressed(), RewardAddress: v.reward.Address(), BondUnits: cfg.Validator.MinBondUnits}
	data, _ := json.Marshal(req)
	fee, err := cfg.FeePolicy.ComputeFee(computeValidatorRegister)
	if err != nil {
		t.Fatal(err)
	}
	tx := protocolv2.NewTransactionV2(cfg.ChainID, protocolv2.TxValidatorRegister, v.operator.Address(), "", cfg.Validator.MinBondUnits, sequence, computeValidatorRegister, fee, data)
	if err := tx.Sign(v.operator); err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestPhase3CValidatorRegistrationRequiresConfirmedNodeIdentity(t *testing.T) {
	cfg := phase3CTestConfig()
	v := newSandboxValidator(t, 0)
	recV2 := NodeRecordV2{NodeRecord: NodeRecord{NodeID: v.nodeID, PeerID: v.peerID, ChainID: int(cfg.ChainID), TEPPubkey: "phase3e-test-tep"}, RecordVersion: NodeRecordVersion2, NodeClass: NodeClassNode, Roles: []NodeRole{NodeRoleFull}, TEPEnabled: true, Capabilities: []string{NodeCapabilityHVMV2, NodeCapabilityValidatorV2, NodeCapabilityConsensusV2}, ProtocolVersion: BlockVersionV2}
	nodes := map[string]ConfirmedNodeIdentity{strings.ToLower(v.nodeID): {NodeID: v.nodeID, PeerID: v.peerID, OperatorAddress: v.operator.Address(), Record: recV2.NodeRecord, RecordV2: &recV2}}
	req := consensus.RegisterRequest{NodeID: v.nodeID, PeerID: v.peerID, ConsensusPubKey: v.consensus.PublicKeyHexCompressed(), RewardAddress: v.reward.Address(), BondUnits: cfg.Validator.MinBondUnits}
	if err := validateValidatorNodeBinding(v.operator.Address(), req, nodes, cfg.ChainID, true); err != nil {
		t.Fatalf("valid node binding rejected: %v", err)
	}
	req.PeerID = "wrong-peer"
	if err := validateValidatorNodeBinding(v.operator.Address(), req, nodes, cfg.ChainID, true); err == nil {
		t.Fatal("mismatched PeerID unexpectedly accepted")
	}
	other, _ := wallet.NewWallet()
	req.PeerID = v.peerID
	if err := validateValidatorNodeBinding(other.Address(), req, nodes, cfg.ChainID, true); err == nil {
		t.Fatal("non-owner operator unexpectedly accepted")
	}

	edge := recV2
	edge.Roles = []NodeRole{NodeRoleEdge}
	edge.Role = ""
	edgeNodes := map[string]ConfirmedNodeIdentity{strings.ToLower(v.nodeID): {NodeID: v.nodeID, PeerID: v.peerID, OperatorAddress: v.operator.Address(), Record: edge.NodeRecord, RecordV2: &edge}}
	if err := validateValidatorNodeBinding(v.operator.Address(), req, edgeNodes, cfg.ChainID, true); err == nil {
		t.Fatal("edge node unexpectedly accepted as validator")
	}

	missingCapability := recV2
	missingCapability.Capabilities = []string{NodeCapabilityHVMV2, NodeCapabilityValidatorV2}
	capNodes := map[string]ConfirmedNodeIdentity{strings.ToLower(v.nodeID): {NodeID: v.nodeID, PeerID: v.peerID, OperatorAddress: v.operator.Address(), Record: missingCapability.NodeRecord, RecordV2: &missingCapability}}
	if err := validateValidatorNodeBinding(v.operator.Address(), req, capNodes, cfg.ChainID, true); err == nil {
		t.Fatal("node missing consensus-v2 capability unexpectedly accepted")
	}
}

func TestPhase3CMultiNodeFinalitySandbox(t *testing.T) {
	cfg := phase3CTestConfig()
	vals := make([]sandboxValidator, 4)
	byID := make(map[string]sandboxValidator, 4)
	for i := range vals {
		vals[i] = newSandboxValidator(t, i)
		byID[strings.ToLower(vals[i].id)] = vals[i]
	}

	nodes := make([]*Blockchain, 4)
	for i := range nodes {
		nodes[i] = NewBlockchainWithDirAndV2Config(t.TempDir(), cfg)
	}
	leader := nodes[0]

	// Heights 1..4: each prospective validator first registers its node identity
	// on the existing DLT and receives one legacy mining reward. This is a
	// confirmed prerequisite; validator registration cannot self-assert NodeID.
	for i, v := range vals {
		leader.SetPendingTransactions([]*Transaction{signedNodeRegistration(t, v, cfg.ChainID)}, nil)
		if err := leader.AddBlock(v.operator.Address()); err != nil {
			t.Fatalf("legacy bootstrap block %d: %v", i+1, err)
		}
		block := cloneBlockForConsensus(leader.Blocks[len(leader.Blocks)-1])
		for n := 1; n < len(nodes); n++ {
			if err := nodes[n].AppendBlock(cloneBlockForConsensus(block)); err != nil {
				t.Fatalf("node %d append bootstrap block %d: %v", n, block.Index, err)
			}
		}
	}

	// Height 5: Protocol V2 active, validator consensus still dark. Four signed
	// registrations escrow 10 HBT each. Consensus keys and reward wallets are
	// separate from the node/operator wallet by registry rule.
	regs := make([]*protocolv2.TransactionV2, 0, 4)
	for _, v := range vals {
		regs = append(regs, validatorRegisterTx(t, cfg, v, 0))
	}
	leader.SetPendingTransactions(nil, regs)
	if err := leader.AddBlock(vals[0].operator.Address()); err != nil {
		t.Fatalf("validator registration block: %v", err)
	}
	b5 := cloneBlockForConsensus(leader.Blocks[len(leader.Blocks)-1])
	for n := 1; n < len(nodes); n++ {
		if err := nodes[n].AppendBlock(cloneBlockForConsensus(b5)); err != nil {
			t.Fatalf("node %d append validator block: %v", n, err)
		}
	}

	for _, v := range vals {
		registered, ok := leader.ValidatorRegistry().Get(v.id)
		if !ok {
			t.Fatalf("validator %s not registered", v.id)
		}
		if leader.BalanceUnits(registered.EscrowAddress) != cfg.Validator.MinBondUnits {
			t.Fatalf("escrow balance for %s=%d want %d", v.id, leader.BalanceUnits(registered.EscrowAddress), cfg.Validator.MinBondUnits)
		}
		if wallet.AddressEqual(registered.ConsensusAddress, registered.RewardAddress) || wallet.AddressEqual(registered.ConsensusAddress, registered.OperatorAddress) {
			t.Fatal("validator key separation invariant broken")
		}
	}

	// Height 6 advances pending validators to ACTIVE, still without finality.
	leader.SetPendingTransactions(nil, nil)
	if err := leader.AddBlock(vals[0].operator.Address()); err != nil {
		t.Fatalf("activation block: %v", err)
	}
	b6 := cloneBlockForConsensus(leader.Blocks[len(leader.Blocks)-1])
	for n := 1; n < len(nodes); n++ {
		if err := nodes[n].AppendBlock(cloneBlockForConsensus(b6)); err != nil {
			t.Fatalf("node %d append activation block: %v", n, err)
		}
	}

	set := leader.CurrentValidatorSet(7)
	if len(set.Validators) != 4 || set.TotalPower() != 4 || consensus.QuorumThreshold(set.TotalPower()) != 3 {
		t.Fatalf("unexpected active set: count=%d power=%d threshold=%d", len(set.Validators), set.TotalPower(), consensus.QuorumThreshold(set.TotalPower()))
	}
	expected, ok := set.Proposer(7, 0)
	if !ok {
		t.Fatal("no proposer")
	}
	wrong := set.Validators[0]
	if strings.EqualFold(wrong.ID, expected.ID) {
		wrong = set.Validators[1]
	}
	if _, err := leader.BuildConsensusProposal(wrong.ID, 0); err == nil {
		t.Fatal("wrong proposer unexpectedly built proposal")
	}

	proposal, err := leader.BuildConsensusProposal(expected.ID, 0)
	if err != nil {
		t.Fatalf("build proposal: %v", err)
	}
	votes := make([]consensus.Vote, 0, 3)
	for i := 0; i < 3; i++ {
		active := set.Validators[i]
		fixture, ok := byID[strings.ToLower(active.ID)]
		if !ok {
			t.Fatalf("missing signer for %s", active.ID)
		}
		vote, err := nodes[i].SignConsensusVote(proposal, active.ID, fixture.consensus)
		if err != nil {
			t.Fatalf("node %d vote validation/signing: %v", i, err)
		}
		votes = append(votes, vote)
	}
	// The local vote journal is an additional safety barrier: after validator 0
	// signed a proposal at height 7 it must not sign a different proposal for
	// that same height in a later round, even after proposer rotation.
	roundOneProposer, ok := set.Proposer(7, 1)
	if !ok {
		t.Fatal("missing round-1 proposer")
	}
	roundOneProposal, err := leader.BuildConsensusProposal(roundOneProposer.ID, 1)
	if err != nil {
		t.Fatalf("build round-1 proposal: %v", err)
	}
	firstActive := set.Validators[0]
	firstFixture := byID[strings.ToLower(firstActive.ID)]
	if _, err := nodes[0].SignConsensusVote(roundOneProposal, firstActive.ID, firstFixture.consensus); err == nil {
		t.Fatal("cross-round double-sign unexpectedly allowed by local vote journal")
	}

	if _, err := consensus.BuildQuorumCertificate(cfg.ChainID, 7, 0, proposal.Hash, proposal.ValidatorSetRoot, set, votes[:2]); err == nil {
		t.Fatal("2-of-4 QC unexpectedly accepted")
	}
	qc, err := consensus.BuildQuorumCertificate(cfg.ChainID, 7, 0, proposal.Hash, proposal.ValidatorSetRoot, set, votes)
	if err != nil {
		t.Fatalf("3-of-4 QC rejected: %v", err)
	}
	if err := leader.FinalizeConsensusProposal(proposal, qc); err != nil {
		t.Fatalf("leader finalize: %v", err)
	}
	finalBlock := cloneBlockForConsensus(leader.Blocks[len(leader.Blocks)-1])
	for n := 1; n < len(nodes); n++ {
		if err := nodes[n].AppendBlock(cloneBlockForConsensus(finalBlock)); err != nil {
			t.Fatalf("node %d rejected finalized block: %v", n, err)
		}
	}

	wantHash := finalBlock.Hash
	wantHBT := finalBlock.HBTStateRoot
	wantHVM := finalBlock.HVMStateRoot
	wantVals := finalBlock.ValidatorStateRoot
	for i, bc := range nodes {
		if bc.Height() != 7 || bc.FinalizedHeight() != 7 {
			t.Fatalf("node %d height/finality=%d/%d want 7/7", i, bc.Height(), bc.FinalizedHeight())
		}
		head := bc.Blocks[len(bc.Blocks)-1]
		if head.Hash != wantHash || head.HBTStateRoot != wantHBT || head.HVMStateRoot != wantHVM || head.ValidatorStateRoot != wantVals {
			t.Fatalf("node %d state diverged", i)
		}
		if bc.ValidatorRegistry().Root() != leader.ValidatorRegistry().Root() {
			t.Fatalf("node %d validator registry root diverged", i)
		}
	}
}
