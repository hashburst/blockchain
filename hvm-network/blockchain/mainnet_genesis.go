package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hashburst/consensus"
	"hashburst/hvm"
)

const MainnetChainID uint64 = 4735489
const mainnetRecipient = "0xd1da8d04d767685e53440dbc56803af350a65333"
const terminalSourceHash = "0000a8bef0916f373a97fa8c311b095258f00cf7f2a24a3ff2f875156ca27ae1"
const freezeEvidenceHash = "d0f2246fc26e9e72c6117ac0d6f6805c7427a8cc1b106613c822a1cbb7e0a6f6"
const legacyImportUnits int64 = 45000000000
const founderAllocationUnits int64 = 100000000000000000

// MainnetGenesisImport is an explicitly pinned allocation, not an RPC import.
// A change to any field requires a new reviewed consensus configuration.
// Freeze evidence attests managed services at observation time, not other forks.
type MainnetGenesisImport struct {
	SourceChainID        uint64 `json:"source_chain_id"`
	SourceHeight         uint64 `json:"source_height"`
	SourceHash           string `json:"source_hash"`
	Recipient            string `json:"recipient"`
	Units                int64  `json:"units"`
	FounderUnits         int64  `json:"founder_units"`
	FreezeEvidenceSHA256 string `json:"freeze_evidence_sha256"`
}

func ApprovedMainnetGenesisImport() MainnetGenesisImport {
	return MainnetGenesisImport{1337, 10, terminalSourceHash, mainnetRecipient, legacyImportUnits, founderAllocationUnits, freezeEvidenceHash}
}
func (m MainnetGenesisImport) Validate(chainID uint64) error {
	if chainID != MainnetChainID || m != ApprovedMainnetGenesisImport() {
		return fmt.Errorf("mainnet genesis import differs from approved economic commitments")
	}
	return nil
}
func (m MainnetGenesisImport) Nullifier() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("HashBurst/legacy-import/v1:%d:%d:%s", m.SourceChainID, m.SourceHeight, m.SourceHash)))
	return hex.EncodeToString(sum[:])
}
func (m MainnetGenesisImport) commitment() string {
	c := newCanonicalBuffer()
	c.putString("HASHBURST_MAINNET_IMPORT_V1")
	c.putUint64(MainnetChainID)
	c.putString(m.Nullifier())
	c.putString(m.Recipient)
	c.putInt64(m.Units)
	c.putInt64(m.FounderUnits)
	c.putString(m.FreezeEvidenceSHA256)
	sum := sha256.Sum256(c.bytes())
	return hex.EncodeToString(sum[:])
}

// applyGenesisImport is package-private and only allowed on an empty state.
// Validation precedes mutations; the lock publishes allocation and nullifier
// together. Persistence comes from the genesis/config plus deterministic replay,
// never from an out-of-band consumed marker or a writable RPC.
func (s *State) applyGenesisImport(chainID uint64, m MainnetGenesisImport) error {
	if err := m.Validate(chainID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.consumedImports) != 0 {
		return fmt.Errorf("genesis import already consumed")
	}
	if len(s.balances) != 0 || len(s.sequences) != 0 || s.evm != nil {
		return fmt.Errorf("genesis import requires pristine state")
	}
	total, err := checkedAddInt64(m.Units, m.FounderUnits)
	if err != nil {
		return err
	}
	s.balances = map[string]int64{stateKey(m.Recipient): total}
	s.consumedImports = map[string]string{m.Nullifier(): m.commitment()}
	return nil
}
func initialProtocolState(cfg ProtocolV2Config) *State {
	s := NewState()
	if cfg.GenesisImport != nil {
		if err := s.applyGenesisImport(cfg.ChainID, *cfg.GenesisImport); err != nil {
			panic(err)
		}
	}
	return s
}
func protocolGenesis(cfg ProtocolV2Config) *Block {
	b := NewGenesisBlock()
	if cfg.GenesisImport == nil {
		return b
	}
	b.PrevHash = recoveryProtocolHash(cfg)
	b.Version = BlockVersionV2
	b.ProtocolChainID = cfg.ChainID
	b.HBTStateRoot = initialProtocolState(cfg).Root()
	b.HVMStateRoot = hvm.NewEngine(nil, cfg.FeePolicy).State().Root()
	registry := consensus.NewRegistry(cfg.Validator)
	b.ValidatorStateRoot = registry.Root()
	// The initial economic checkpoint does not contain a validator set. A
	// production bootstrap must register/bond the approved fresh identities.
	b.Hash = b.GenerateHash()
	return b
}
func validProtocolGenesis(b *Block, cfg ProtocolV2Config) bool {
	if b == nil {
		return false
	}
	expected := protocolGenesis(cfg)
	if cfg.GenesisImport == nil {
		return b.Hash == expected.Hash
	}
	// Exact codec comparison rejects uncommitted/ignored genesis metadata too.
	disk := blockToOnDisk(b)
	canonical := blockFromOnDisk(&disk)
	expectedDisk := blockToOnDisk(expected)
	expected = blockFromOnDisk(&expectedDisk)
	left, e := EncodeLedgerBlock(nil, canonical)
	if e != nil {
		return false
	}
	right, e := EncodeLedgerBlock(nil, expected)
	if e != nil {
		return false
	}
	return string(left) == string(right)
}

// MainnetEconomicGenesis returns the deterministic economic checkpoint. It has
// no active validators and MUST NOT be treated as runtime activation acceptance.
func MainnetEconomicGenesis(cfg ProtocolV2Config) (*Block, error) {
	if cfg.GenesisImport == nil {
		return nil, fmt.Errorf("explicit mainnet genesis import required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return protocolGenesis(cfg), nil
}
