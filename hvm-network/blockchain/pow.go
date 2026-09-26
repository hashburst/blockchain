package blockchain

import "fmt"

// Difficulty: numero di zeri esadecimali iniziali richiesti.
// 4 -> 1 tentativo valido su 65536 -> ~65k hash per blocco -> decine di ms.
const Difficulty = 4

// maxNonce limita la ricerca. Con Difficulty=4 la probabilita' di non trovare
// un nonce valido entro 10 milioni di tentativi e' ~e^-152: se accade, e' un
// bug, non sfortuna, e va segnalato invece di ciclare.
const maxNonce = 10_000_000

// MineBlock cerca un nonce che soddisfi il PoW e fissa b.Hash.
//
// IL BUG PRECEDENTE: il ciclo era `for { ... b.ProofOfWork++ }` senza uscita, e
// il nonce entrava nell'hash via string(rune(b.ProofOfWork)). Superati 1.114.111
// tentativi quella conversione restituisce sempre U+FFFD: l'hash smetteva di
// cambiare e il ciclo non poteva piu' terminare. Con Difficulty=4 succedeva
// circa una volta ogni 24 milioni di blocchi, e in quel caso il processo
// bruciava una CPU per sempre.
//
// Il vecchio corpo conteneva anche:
//
//	_ = fmt.Sprintf("%d", b.ProofOfWork) // evita import non usato
//	_ = sha256.New()
//	_ = hex.EncodeToString(nil)
//
// Tre allocazioni per iterazione — 65.000 volte per blocco — al solo scopo di
// zittire "imported and not used". La soluzione era togliere gli import.
func (b *Block) MineBlock() error {
	return b.MineBlockWithDifficulty(Difficulty)
}

// MineBlockWithDifficulty is used by config-aware legacy/pre-consensus paths.
// Consensus-active BFT blocks never call this function: their authority comes
// from scheduled proposer signatures and quorum finality, not brute-force PoW.
func (b *Block) MineBlockWithDifficulty(difficulty int) error {
	if err := validatePoWDifficulty(difficulty); err != nil {
		return fmt.Errorf("mining difficulty: %w", err)
	}
	for nonce := int64(0); nonce < maxNonce; nonce++ {
		b.ProofOfWork = nonce
		hash := b.GenerateHash()
		if MeetsDifficultyAt(hash, difficulty) {
			b.Hash = hash
			return nil
		}
	}
	return fmt.Errorf("mining: nessun nonce valido entro %d tentativi (difficulty=%d)",
		maxNonce, difficulty)
}
