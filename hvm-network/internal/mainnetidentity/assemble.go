package mainnetidentity

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"hashburst/blockchain"
	"hashburst/consensus"
	"hashburst/ledger"
	"hashburst/protocolv2"
)

// Assemble executes signed funding and registrations in an exclusive offline
// directory. CHECKPOINT.json is written only after full replay reconciliation.
// Failed partial directories are retained and cannot be provisioned as complete.
func Assemble(out, economicLedger string, c blockchain.ProtocolV2Config, identities []Identity, signed []*protocolv2.TransactionV2) error {
	if c.MainnetBootstrapEnd == 0 {
		return fmt.Errorf("explicit zero-issuance bootstrap schedule required")
	}
	if e := VerifyFunding(c, identities, signed); e != nil {
		return e
	}
	p, e := PlanFunding(c, identities)
	if e != nil {
		return e
	}
	g, e := blockchain.MainnetEconomicGenesis(c)
	if e != nil {
		return e
	}
	source, e := blockchain.OpenExistingBlockchain(economicLedger, c, g.Hash, 0, g.Hash)
	if e != nil {
		return e
	}
	height := source.Height()
	e = source.CloseHistory()
	if e != nil {
		return e
	}
	if height != 0 {
		return fmt.Errorf("pristine economic checkpoint required")
	}
	if e = newDir(out); e != nil {
		return e
	}
	dir := filepath.Join(out, ".assembly")
	bc := blockchain.NewBlockchainWithDirAndV2Config(dir, c)
	defer bc.CloseHistory()
	var registrations []*blockchain.Transaction
	var validators []*protocolv2.TransactionV2
	for _, v := range identities {
		registrations = append(registrations, v.Registration)
		validators = append(validators, v.Validator)
	}
	bc.SetPendingTransactions(registrations, signed)
	if e = bc.AddBlock(p.Founder); e != nil {
		return e
	}
	for _, tx := range signed {
		r, ok := bc.Receipt(tx.HashHex())
		if !ok || !r.Success {
			return fmt.Errorf("funding execution failed")
		}
	}
	bc.SetPendingTransactions(nil, validators)
	if e = bc.AddBlock(p.Founder); e != nil {
		return e
	}
	for _, tx := range validators {
		r, ok := bc.Receipt(tx.HashHex())
		if !ok || !r.Success {
			return fmt.Errorf("registration execution failed")
		}
	}
	bc.SetPendingTransactions(nil, nil)
	for uint64(bc.Height()) < c.MainnetBootstrapEnd {
		if e = bc.AddBlock(p.Founder); e != nil {
			return e
		}
	}
	set := bc.CurrentValidatorSet(c.ConsensusActivationHeight)
	if len(set.Validators) != len(identities) {
		return fmt.Errorf("active validator count differs")
	}
	expected := map[string]int64{strings.ToLower(p.Founder): c.GenesisImport.Units + p.RemainingFounderAllocation}
	expected[strings.ToLower(c.FeeCollector)] += p.FeeUnits
	for _, v := range identities {
		_, q, e := validate(c, v)
		if e != nil {
			return e
		}
		id, e := consensus.ValidatorID(q.ConsensusPubKey)
		if e != nil {
			return e
		}
		expected[strings.ToLower(consensus.ValidatorEscrowAddress(id))] += q.BondUnits
	}
	reconcile := func(chain *blockchain.Blockchain) error {
		sum := new(big.Int)
		for addr, amount := range chain.State().Snapshot() {
			if amount != expected[strings.ToLower(addr)] {
				return fmt.Errorf("unexpected balance for %s", addr)
			}
			sum.Add(sum, big.NewInt(amount))
		}
		for addr, amount := range expected {
			if chain.BalanceUnits(addr) != amount {
				return fmt.Errorf("expected balance missing")
			}
		}
		if sum.Cmp(big.NewInt(c.GenesisImport.Units+c.GenesisImport.FounderUnits)) != 0 {
			return fmt.Errorf("supply changed")
		}
		return nil
	}
	if e = reconcile(bc); e != nil {
		return e
	}
	head, e := bc.BlockAt(bc.Height())
	if e != nil {
		return e
	}
	root := bc.HBTStateRoot()
	published := filepath.Join(out, "ledger")
	if _, e = ledger.WriteGeneration(published, uint64(head.Index+1), func(n uint64) ([]byte, error) {
		b, e := bc.BlockAt(int(n))
		if e != nil {
			return nil, e
		}
		return blockchain.EncodeLedgerBlock(nil, b)
	}); e != nil {
		return e
	}
	if e = bc.CloseHistory(); e != nil {
		return e
	}
	reopened, e := blockchain.OpenExistingBlockchain(published, c, g.Hash, head.Index, head.Hash)
	if e != nil {
		return e
	}
	defer reopened.CloseHistory()
	if e = reconcile(reopened); e != nil {
		return e
	}
	if reopened.HBTStateRoot() != root || reopened.CurrentValidatorSet(c.ConsensusActivationHeight).Root() != set.Root() {
		return fmt.Errorf("replay commitments differ")
	}
	if e = jsonFile(filepath.Join(out, "protocol.json"), c); e != nil {
		return e
	}
	if e = jsonFile(filepath.Join(out, "enrollments.json"), identities); e != nil {
		return e
	}
	if e = jsonFile(filepath.Join(out, "funding.json"), signed); e != nil {
		return e
	}
	if e = jsonFile(filepath.Join(out, "CHECKPOINT.json"), map[string]any{
		"schema": "hashburst-mainnet-validator-checkpoint-v1", "activation_allowed": false, "runtime_acceptance_required": true,
		"chain_id": c.ChainID, "genesis_hash": g.Hash, "checkpoint_height": head.Index, "checkpoint_hash": head.Hash,
		"protocol_sha256": protocolHash(c), "native_state_root": root, "validator_set": set, "source_nullifier": c.GenesisImport.Nullifier(),
		"bootstrap_issuance_units": 0, "total_units": c.GenesisImport.Units + c.GenesisImport.FounderUnits,
	}); e != nil {
		return e
	}
	d, e := os.Open(out)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
