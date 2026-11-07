package blockchain

import (
    "encoding/json"
    "os"
    "testing"
)

func TestMainnetProductionFreezeCandidateValidates(t *testing.T) {
    raw, err := os.ReadFile("../deploy/mainnet/protocol.freeze.json")
    if err != nil { t.Fatal(err) }
    var cfg ProtocolV2Config
    if err = json.Unmarshal(raw, &cfg); err != nil { t.Fatal(err) }
    if err = cfg.Validate(); err != nil { t.Fatalf("production freeze rejected: %v", err) }
    if cfg.ChainID != MainnetChainID || cfg.MainnetBootstrapEnd != 8 || cfg.ConsensusActivationHeight != 9 {
        t.Fatal("unexpected production freeze boundary")
    }
    if cfg.GenesisImport == nil || *cfg.GenesisImport != ApprovedMainnetGenesisImport() {
        t.Fatal("production freeze genesis import differs")
    }
    if cfg.EffectivePoHTicks() != PoHTicks {
        t.Fatal("production freeze must retain historical PoH ticks")
    }
    if cfg.APoW == nil || cfg.APoW.TargetSeconds != 60 || cfg.APoW.Window != 60 {
        t.Fatal("unexpected APoW production target")
    }
}
