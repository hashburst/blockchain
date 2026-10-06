package mainnetidentity

import (
	"bytes"
	"fmt"
	"hashburst/blockchain"
	"hashburst/hvm"
	"hashburst/protocolv2"
	"math"
	"sort"
	"strings"
)

// FundingPlan is a deterministic proposal for pristine genesis, not live state.
type FundingPlan struct {
	ProtocolHash               string                      `json:"protocol_sha256"`
	SourceNullifier            string                      `json:"source_nullifier"`
	Founder                    string                      `json:"founder"`
	DebitedUnits               int64                       `json:"debited_units"`
	BondUnits                  int64                       `json:"bond_units"`
	FeeUnits                   int64                       `json:"fee_units"`
	RemainingFounderAllocation int64                       `json:"remaining_founder_allocation_units"`
	Transfers                  []*protocolv2.TransactionV2 `json:"unsigned_transfers"`
}

// PlanFunding allocates bonds and fees only from the founder allocation.
// It verifies all enrollments and does not sign, credit or submit transactions.
func PlanFunding(c blockchain.ProtocolV2Config, identities []Identity) (*FundingPlan, error) {
	if e := validateConfig(c); e != nil {
		return nil, e
	}
	if len(identities) < 4 || len(identities) > 6 {
		return nil, fmt.Errorf("4..6 validator enrollments required")
	}
	list := append([]Identity(nil), identities...)
	seen := map[string]bool{}
	for _, v := range list {
		if e := Verify(c, v); e != nil {
			return nil, e
		}
		r, q, e := validate(c, v)
		if e != nil {
			return nil, e
		}
		for _, key := range []string{"node:" + r.NodeID, "peer:" + r.PeerID, "consensus:" + strings.ToLower(q.ConsensusPubKey), "operator:" + strings.ToLower(v.Validator.Sender), "tep:" + strings.ToLower(r.TEPPubkey), "endpoint:" + strings.Split(r.Multiaddrs[0], "/p2p/")[0]} {
			if seen[key] {
				return nil, fmt.Errorf("duplicate enrollment binding")
			}
			seen[key] = true
		}
		if strings.EqualFold(v.Validator.Sender, c.GenesisImport.Recipient) {
			return nil, fmt.Errorf("operator must differ from founder")
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Validator.Sender) < strings.ToLower(list[j].Validator.Sender)
	})
	transferFee, e := c.FeePolicy.ComputeFee(hvm.ComputeBaseTransfer)
	if e != nil {
		return nil, e
	}
	registrationFee, e := c.FeePolicy.ComputeFee(compute)
	if e != nil {
		return nil, e
	}
	bond := c.Validator.MinBondUnits
	if bond <= 0 || transferFee < 0 || registrationFee < 0 || bond > math.MaxInt64-registrationFee {
		return nil, fmt.Errorf("invalid funding amounts")
	}
	amount := bond + registrationFee
	if amount > math.MaxInt64-transferFee {
		return nil, fmt.Errorf("funding overflow")
	}
	perNode := amount + transferFee
	if perNode > c.GenesisImport.FounderUnits/int64(len(list)) {
		return nil, fmt.Errorf("insufficient founder allocation")
	}
	p := &FundingPlan{ProtocolHash: protocolHash(c), SourceNullifier: c.GenesisImport.Nullifier(), Founder: c.GenesisImport.Recipient}
	for i, v := range list {
		p.Transfers = append(p.Transfers, protocolv2.NewTransactionV2(c.ChainID, protocolv2.TxHBTTransfer, p.Founder, v.Validator.Sender, amount, uint64(i), hvm.ComputeBaseTransfer, transferFee, nil))
		p.DebitedUnits += perNode
		p.BondUnits += bond
		p.FeeUnits += transferFee + registrationFee
	}
	p.RemainingFounderAllocation = c.GenesisImport.FounderUnits - p.DebitedUnits
	if p.DebitedUnits != p.BondUnits+p.FeeUnits {
		return nil, fmt.Errorf("funding reconciliation failed")
	}
	return p, nil
}

// VerifyFunding recomputes the plan instead of trusting submitted totals.
// Runtime replay must separately enforce account sequences and balances.
func VerifyFunding(c blockchain.ProtocolV2Config, identities []Identity, signed []*protocolv2.TransactionV2) error {
	p, e := PlanFunding(c, identities)
	if e != nil {
		return e
	}
	if len(signed) != len(p.Transfers) {
		return fmt.Errorf("funding transfer count differs")
	}
	for i, tx := range signed {
		if tx == nil {
			return fmt.Errorf("nil funding transfer")
		}
		if !bytes.Equal(tx.CanonicalBytes(), p.Transfers[i].CanonicalBytes()) {
			return fmt.Errorf("funding transfer %d differs", i)
		}
		if e := tx.Verify(c.ChainID); e != nil {
			return fmt.Errorf("funding signature %d: %w", i, e)
		}
	}
	return nil
}
