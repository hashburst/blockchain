package hvm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// StandardsRegistry is intentionally open-ended: HBT-20/HBT-721 are merely
// initial profiles. New ERC-equivalent application profiles can be registered
// without hard-coding every future standard in the HVM core.
type StandardsRegistry struct {
	mu        sync.RWMutex
	standards map[string]StandardDescriptor
}

func NewStandardsRegistry() *StandardsRegistry {
	r := &StandardsRegistry{standards: make(map[string]StandardDescriptor)}
	for _, d := range DefaultStandardCatalog() {
		_ = r.Register(d)
	}
	return r
}

func (r *StandardsRegistry) Register(d StandardDescriptor) error {
	d.ID = strings.ToUpper(strings.TrimSpace(d.ID))
	d.Origin = strings.ToUpper(strings.TrimSpace(d.Origin))
	if d.ID == "" || d.Origin == "" {
		return fmt.Errorf("standard id and origin are required")
	}
	if d.Runtime != RuntimeNative && d.Runtime != RuntimeEVM {
		return fmt.Errorf("unsupported runtime %q", d.Runtime)
	}
	if d.Status == "" {
		d.Status = StandardAvailable
	}
	if d.Status != StandardAvailable && d.Status != StandardActive && d.Status != StandardDeprecated {
		return fmt.Errorf("unsupported standard status %q", d.Status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.standards[d.ID]; exists {
		return fmt.Errorf("standard %s already registered", d.ID)
	}
	r.standards[d.ID] = d
	return nil
}

func (r *StandardsRegistry) Get(id string) (StandardDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.standards[strings.ToUpper(strings.TrimSpace(id))]
	return d, ok
}

func (r *StandardsRegistry) List() []StandardDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.standards))
	for id := range r.standards {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]StandardDescriptor, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.standards[id])
	}
	return out
}

func DefaultStandardCatalog() []StandardDescriptor {
	mk := func(id, origin string, runtime Runtime, caps []string, deps ...string) StandardDescriptor {
		return StandardDescriptor{
			ID: id, Origin: origin, Version: "draft-1", Runtime: runtime,
			Capabilities: caps, Dependencies: deps, Status: StandardAvailable,
		}
	}
	return []StandardDescriptor{
		mk("HBT-165", "ERC-165", RuntimeEVM, []string{"interface-discovery"}),
		mk("HBT-20", "ERC-20", RuntimeEVM, []string{"fungible-token"}),
		mk("HBT-2612", "ERC-2612", RuntimeEVM, []string{"permit"}, "HBT-20"),
		mk("HBT-721", "ERC-721", RuntimeEVM, []string{"non-fungible-token"}, "HBT-165"),
		mk("HBT-1155", "ERC-1155", RuntimeEVM, []string{"multi-token"}, "HBT-165"),
		mk("HBT-1271", "ERC-1271", RuntimeEVM, []string{"contract-signatures"}),
		mk("HBT-2771", "ERC-2771", RuntimeEVM, []string{"meta-transactions"}),
		mk("HBT-2981", "ERC-2981", RuntimeEVM, []string{"royalties"}, "HBT-165"),
		mk("HBT-3156", "ERC-3156", RuntimeEVM, []string{"flash-lending"}),
		mk("HBT-3525", "ERC-3525", RuntimeEVM, []string{"semi-fungible-token"}, "HBT-721"),
		mk("HBT-3643", "ERC-3643", RuntimeEVM, []string{"permissioned-token", "rwa"}, "HBT-20"),
		mk("HBT-4337", "ERC-4337", RuntimeEVM, []string{"account-abstraction"}),
		mk("HBT-4626", "ERC-4626", RuntimeEVM, []string{"tokenized-vault"}, "HBT-20"),
		mk("HBT-6909", "ERC-6909", RuntimeEVM, []string{"minimal-multi-token"}),
		{
			ID: "HBT-MINING-PAYOUT-REGISTRY", Origin: "HASHBURST-NATIVE",
			Version: "3.0-draft", Runtime: RuntimeNative,
			Capabilities: []string{"audit-ledger", "multi-chain-settlement-evidence"},
			Status:       StandardActive,
		},
	}
}
