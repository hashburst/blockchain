package blockchain

import "testing"

func TestMainnetImportAcceptsHistoricalPersistedGenesis(t *testing.T) {
	cfg := importFixtureConfig()
	legacy := NewGenesisBlock()

	if !validProtocolGenesis(legacy, cfg) {
		t.Fatal("approved historical persisted genesis rejected")
	}
}

func TestMainnetImportRejectsTamperedHistoricalPersistedGenesis(t *testing.T) {
	cfg := importFixtureConfig()

	tests := []struct {
		name   string
		mutate func(*Block)
	}{
		{
			name: "declared-hash",
			mutate: func(b *Block) {
				b.Hash = "00"
			},
		},
		{
			name: "previous-hash",
			mutate: func(b *Block) {
				b.PrevHash = "tampered"
				b.Hash = b.GenerateHash()
			},
		},
		{
			name: "timestamp",
			mutate: func(b *Block) {
				b.Timestamp = b.Timestamp.Add(1)
				b.Hash = b.GenerateHash()
			},
		},
		{
			name: "proof-of-time",
			mutate: func(b *Block) {
				b.ProofOfTime++
				b.Hash = b.GenerateHash()
			},
		},
		{
			name: "proof-of-work",
			mutate: func(b *Block) {
				b.ProofOfWork++
				b.Hash = b.GenerateHash()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewGenesisBlock()
			tt.mutate(b)

			if validProtocolGenesis(b, cfg) {
				t.Fatal("tampered historical genesis accepted")
			}
		})
	}
}

func TestMainnetImportStillAcceptsEconomicGenesis(t *testing.T) {
	cfg := importFixtureConfig()

	economic, err := MainnetEconomicGenesis(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !validProtocolGenesis(economic, cfg) {
		t.Fatal("canonical economic genesis rejected")
	}
}
