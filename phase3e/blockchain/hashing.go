package blockchain

// hashing.go — costanti di catena e utilità per il PoW.
//
// Le funzioni di codifica canonica (putInt64/putUint64/putString) NON sono più
// qui: sono diventate metodi di canonicalBuffer in canonical.go, così le usano
// sia block.go sia transaction.go senza duplicazione. Le regole restano quelle:
//
//   - interi: big-endian a 8 byte, mai come testo
//   - stringhe: prefisso di lunghezza a 8 byte, poi i byte grezzi
//   - importi: interi in unità da 1e-8 HBT, mai float
//   - tempo: UnixNano int64, mai la rappresentazione testuale
//   - ChainID in testa a ogni preimage: blocchi e transazioni di una chain
//     non sono replicabili su un'altra

import (
	"fmt"
	"math"
	"math/big"
)

const (
	// ChainID entra in ogni preimage. Cambiarlo invalida l'intera catena.
	ChainID = "hashburst-mainnet-1"

	// Genesis deterministico: ogni nodo che parte da zero ottiene lo stesso
	// hash genesis. Con time.Now() era impossibile.
	GenesisTimestampNs int64 = 1750000000000000000 // 2025-06-15T15:06:40Z, fisso
	GenesisPrevHash          = "0"

	// AmountScale: 1 HBT = 1e8 unità intere. I float non entrano mai
	// nell'hashing: 0.1+0.2 != 0.3 in binario.
	AmountScale = 1e8

	// DefaultMiningReward: reward per blocco. Costante di consenso, usata anche
	// dai validatori che non hanno un'istanza Blockchain (es. validateFullChain).
	DefaultMiningReward = 50.0
)

// AmountToUnits converte un importo in unità intere.
// math.Round e non un cast: int64(0.29*1e8) darebbe 28999999.
func AmountToUnits(a float64) int64 {
	return int64(math.Round(a * AmountScale))
}

// UnitsToAmount è l'inversa.
func UnitsToAmount(u int64) float64 {
	return float64(u) / AmountScale
}

// powTargetForDifficulty returns the target for the legacy PoW path. The
// production default remains Difficulty=4; dev/test networks may select a lower
// difficulty explicitly through ProtocolV2Config without changing mainnet rules.
func powTargetForDifficulty(difficulty int) *big.Int {
	return new(big.Int).Lsh(big.NewInt(1), uint(256-difficulty*4))
}

func validatePoWDifficulty(difficulty int) error {
	if difficulty < 1 || difficulty > 63 {
		return fmt.Errorf("must be between 1 and 63 hexadecimal nibbles")
	}
	return nil
}

// MeetsDifficultyAt verifies legacy PoW using the configured network difficulty.
func MeetsDifficultyAt(hexHash string, difficulty int) bool {
	if validatePoWDifficulty(difficulty) != nil {
		return false
	}
	hashInt, ok := new(big.Int).SetString(hexHash, 16)
	if !ok {
		return false
	}
	return hashInt.Cmp(powTargetForDifficulty(difficulty)) < 0
}

// MeetsDifficulty preserves the historical/default consensus rule for callers
// that do not carry ProtocolV2Config.
func MeetsDifficulty(hexHash string) bool {
	return MeetsDifficultyAt(hexHash, Difficulty)
}
