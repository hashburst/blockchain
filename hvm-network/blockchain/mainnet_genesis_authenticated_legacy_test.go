package blockchain

import (
	"testing"
	"time"
)

func historicalLegacyGenesisFixture() *Block {
	b := &Block{
		Version:      BlockVersionLegacy,
		Index:        0,
		Timestamp:    time.Unix(0, GenesisTimestampNs-1),
		Transactions: []*Transaction{},
		PrevHash:     "0",
		ProofOfWork:  0,
		ProofOfTime:  0,
	}
	b.Hash = b.GenerateHash()
	return b
}

func TestMainnetImportAcceptsAuthenticatedHistoricalLegacyGenesis(t *testing.T) {
	historical := historicalLegacyGenesisFixture()

	if historical.Hash == NewGenesisBlock().Hash {
		t.Fatal("fixture unexpectedly equals modern deterministic genesis")
	}

	if !validAuthenticatedHistoricalGenesis(historical, historical.Hash) {
		t.Fatal("authenticated historical legacy genesis rejected")
	}
}

func TestMainnetImportRejectsMalformedHistoricalLegacyGenesis(t *testing.T) {
	historical := historicalLegacyGenesisFixture()

	authenticated := historical.Hash
	historical.Hash = "977b557f6be840d9e6c9fadc7e1eb0bb6e1b7a49f228bbc3d04d96cb222cd97b"

	if historical.GenerateHash() == historical.Hash {
		t.Fatal("fixture unexpectedly has valid intrinsic hash")
	}

	if validAuthenticatedHistoricalGenesis(historical, authenticated) {
		t.Fatal("legacy genesis with invalid intrinsic hash accepted")
	}
}

func TestMainnetImportRejectsLegacyGenesisWithV2Metadata(t *testing.T) {
	historical := historicalLegacyGenesisFixture()

	historical.ProtocolChainID = MainnetChainID
	historical.Hash = historical.GenerateHash()

	if validAuthenticatedHistoricalGenesis(historical, historical.Hash) {
		t.Fatal("legacy genesis carrying V2 metadata accepted")
	}
}

func TestAuthenticatedHistoricalGenesisRequiresPinnedIdentity(t *testing.T) {
	historical := historicalLegacyGenesisFixture()

	if validAuthenticatedHistoricalGenesis(
		historical,
		"977b557f6be840d9e6c9fadc7e1eb0bb6e1b7a49f228bbc3d04d96cb222cd97b",
	) {
		t.Fatal("historical genesis accepted under different authenticated identity")
	}
}

func TestAuthenticatedHistoricalGenesisUsesPreviouslyPinnedDiskIdentity(t *testing.T) {
	historical := historicalLegacyGenesisFixture()
	const pinned = "977b557f6be840d9e6c9fadc7e1eb0bb6e1b7a49f228bbc3d04d96cb222cd97b"

	// OpenExistingBlockchain authenticates block zero against this configured
	// identity before validAuthenticatedHistoricalGenesis is reached. The
	// compatibility helper must not reinterpret that authenticated historical
	// identity through the current legacy hash implementation.
	historical.Hash = pinned
	if historical.GenerateHash() == pinned {
		t.Fatal("fixture unexpectedly reproduces pinned historical identity")
	}
	if !validAuthenticatedHistoricalGenesis(historical, pinned) {
		t.Fatal("previously authenticated disk genesis rejected by current hash implementation")
	}
}
