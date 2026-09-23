package blockchain

import (
	"encoding/json"
	"fmt"
	"strings"

	"hashburst/consensus"
	"hashburst/wallet"
)

// ConfirmedNodeIdentity is the deterministic projection of the latest
// NODE_REGISTRATION for one NodeID. Validator admission binds the validator
// operator to this already-confirmed node identity. NODE_REGISTRATION_V2 keeps
// the physical/virtual node class (node|hpc) separate from the node's concurrent
// operational roles (full/blockchain/storage/edge) and TEP capability.
type ConfirmedNodeIdentity struct {
	NodeID             string        `json:"node_id"`
	PeerID             string        `json:"peer_id"`
	OperatorAddress    string        `json:"operator_address"`
	Record             NodeRecord    `json:"record"`
	RecordV2           *NodeRecordV2 `json:"record_v2,omitempty"`
	RegistrationHeight uint64        `json:"registration_height"`
	RegistrationTxID   string        `json:"registration_txid"`
}

func cloneNodeIdentityMap(in map[string]ConfirmedNodeIdentity) map[string]ConfirmedNodeIdentity {
	out := make(map[string]ConfirmedNodeIdentity, len(in))
	for k, v := range in {
		v.Record.Multiaddrs = append([]string(nil), v.Record.Multiaddrs...)
		if v.RecordV2 != nil {
			c := *v.RecordV2
			c.Multiaddrs = append([]string(nil), v.RecordV2.Multiaddrs...)
			c.Capabilities = append([]string(nil), v.RecordV2.Capabilities...)
			v.RecordV2 = &c
		}
		out[k] = v
	}
	return out
}

func applyNodeRegistrations(dst map[string]ConfirmedNodeIdentity, b *Block, expectedChainID uint64) {
	if dst == nil || b == nil {
		return
	}
	for _, tx := range b.Transactions {
		if tx == nil || tx.Receiver != RegistryAddress || tx.Data == "" || !wallet.IsValidAddress(tx.Sender) {
			continue
		}
		var rec NodeRecord
		if err := json.Unmarshal([]byte(tx.Data), &rec); err != nil {
			continue
		}
		if strings.TrimSpace(rec.NodeID) == "" || strings.TrimSpace(rec.PeerID) == "" || rec.ChainID <= 0 || uint64(rec.ChainID) != expectedChainID {
			continue
		}
		var recV2 *NodeRecordV2
		var probe struct {
			RecordVersion int `json:"record_version"`
		}
		if json.Unmarshal([]byte(tx.Data), &probe) == nil && probe.RecordVersion == NodeRecordVersion2 {
			var parsed NodeRecordV2
			if err := json.Unmarshal([]byte(tx.Data), &parsed); err == nil {
				recV2 = &parsed
			}
		}
		key := strings.ToLower(strings.TrimSpace(rec.NodeID))
		dst[key] = ConfirmedNodeIdentity{
			NodeID: strings.TrimSpace(rec.NodeID), PeerID: strings.TrimSpace(rec.PeerID),
			OperatorAddress: tx.Sender, Record: rec, RecordV2: recV2, RegistrationHeight: uint64(b.Index), RegistrationTxID: tx.ID,
		}
	}
}

func nodeIdentityProjection(blocks []*Block, expectedChainID uint64) map[string]ConfirmedNodeIdentity {
	out := make(map[string]ConfirmedNodeIdentity)
	for _, b := range blocks {
		applyNodeRegistrations(out, b, expectedChainID)
	}
	return out
}

func validateValidatorNodeBinding(operator string, req consensus.RegisterRequest, nodes map[string]ConfirmedNodeIdentity, expectedChainID uint64, requireV2 bool) error {
	id, ok := nodes[strings.ToLower(strings.TrimSpace(req.NodeID))]
	if !ok {
		return fmt.Errorf("validator node_id %q has no confirmed NODE_REGISTRATION", req.NodeID)
	}
	if !wallet.AddressEqual(operator, id.OperatorAddress) {
		return fmt.Errorf("validator operator %s does not control registered node %s (owner %s)", operator, req.NodeID, id.OperatorAddress)
	}
	if strings.TrimSpace(req.PeerID) == "" || strings.TrimSpace(id.PeerID) != strings.TrimSpace(req.PeerID) {
		return fmt.Errorf("validator peer_id does not match confirmed NODE_REGISTRATION for %s", req.NodeID)
	}
	if id.Record.ChainID <= 0 || uint64(id.Record.ChainID) != expectedChainID {
		return fmt.Errorf("registered node chain_id %d does not match protocol chain_id %d", id.Record.ChainID, expectedChainID)
	}
	if requireV2 {
		if id.RecordV2 == nil {
			return fmt.Errorf("validator node %s requires confirmed NODE_REGISTRATION_V2", req.NodeID)
		}
		if err := id.RecordV2.ValidateForValidator(expectedChainID); err != nil {
			return fmt.Errorf("validator node capability check: %w", err)
		}
	}
	return nil
}
