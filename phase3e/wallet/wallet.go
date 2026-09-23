package wallet

// wallet.go — chiavi e indirizzi compatibili EVM.
//
// COSA CAMBIA RISPETTO ALLA VERSIONE PRECEDENTE
//
//	prima: ecdsa.GenerateKey(elliptic.P256())  →  sha256(X‖Y)  →  64 hex
//	ora:   secp256k1                           →  keccak256(X‖Y)[12:]  →  0x + 40 hex
//
// Non e' una riformattazione: P-256 e secp256k1 sono curve diverse, e una
// chiave dell'una non puo' produrre un indirizzo dell'altra. Erano due spazi
// di indirizzamento disgiunti, e nessuna chiave del vecchio schema poteva
// firmare una spesa da un indirizzo 0x. Si cambia ora perche' la catena ha 2
// blocchi: fra un anno sarebbe una migrazione di saldi.
//
// DIPENDENZE: nessun modulo nuovo.
//   - secp256k1/v4 era gia' in go.sum come dipendenza indiretta di libp2p
//   - golang.org/x/crypto era gia' diretta, e contiene sha3 (quindi Keccak)
// Serve solo che `go mod tidy` le promuova a dirette.
//
// ATTENZIONE — Keccak-256 NON e' SHA3-256. Sono due funzioni diverse: Ethereum
// usa la versione pre-standardizzazione, con padding differente. In Go e'
// sha3.NewLegacyKeccak256(). Usare sha3.New256() produrrebbe indirizzi
// plausibili e sbagliati, incompatibili con l'intero ecosistema.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secpecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

// AddressLength: 20 byte, come EVM.
const AddressLength = 20

// Wallet custodisce una chiave privata secp256k1.
type Wallet struct {
	priv *secp256k1.PrivateKey
}

// NewWallet genera un wallet nuovo.
//
// FIRMA CAMBIATA: ritorna un errore. Prima era
//
//	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
//
// con l'errore scartato: se la generazione fosse fallita, privKey sarebbe stata
// nil e il nodo sarebbe morto piu' tardi con un nil dereference, lontano dalla
// causa. Una chiave che non si riesce a generare non e' un caso da ignorare.
func NewWallet() (*Wallet, error) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generazione chiave: %w", err)
	}
	return &Wallet{priv: priv}, nil
}

// FromPrivateKeyBytes ricostruisce un wallet da 32 byte.
func FromPrivateKeyBytes(b []byte) (*Wallet, error) {
	if len(b) != 32 {
		return nil, fmt.Errorf("chiave privata: attesi 32 byte, ricevuti %d", len(b))
	}
	priv := secp256k1.PrivKeyFromBytes(b)
	if priv.Key.IsZero() {
		return nil, errors.New("chiave privata nulla")
	}
	return &Wallet{priv: priv}, nil
}

// FromPrivateKeyHex accetta con o senza prefisso 0x.
func FromPrivateKeyHex(s string) (*Wallet, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("chiave privata non esadecimale: %w", err)
	}
	return FromPrivateKeyBytes(b)
}

// PrivateKeyBytes: 32 byte. Da non loggare, mai.
func (w *Wallet) PrivateKeyBytes() []byte { return w.priv.Serialize() }

// PublicKeyBytes: 65 byte non compressi (0x04 ‖ X ‖ Y).
func (w *Wallet) PublicKeyBytes() []byte { return w.priv.PubKey().SerializeUncompressed() }

// Keccak256 — quello di Ethereum, non SHA3-256. Vedi la nota in testa al file.
func Keccak256(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum(nil)
}

// AddressBytes: keccak256 della pubkey senza il prefisso 0x04, ultimi 20 byte.
func (w *Wallet) AddressBytes() []byte {
	pub := w.priv.PubKey().SerializeUncompressed()
	return Keccak256(pub[1:])[12:]
}

// Address: indirizzo con checksum EIP-55.
func (w *Wallet) Address() string { return ToChecksumAddress(w.AddressBytes()) }

// ToChecksumAddress applica il checksum EIP-55.
//
// Il checksum vive nel MAIUSCOLO/minuscolo delle lettere a-f: si calcola il
// keccak dell'indirizzo in minuscolo, e la lettera i-esima va maiuscola se il
// nibble i-esimo di quell'hash e' >= 8. Un indirizzo con una cifra sbagliata
// fallisce la verifica invece di mandare i fondi nel vuoto.
func ToChecksumAddress(addr []byte) string {
	lower := hex.EncodeToString(addr)
	hash := Keccak256([]byte(lower))
	out := []byte("0x" + lower)
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c < 'a' || c > 'f' {
			continue // le cifre 0-9 non portano checksum
		}
		nibble := hash[i/2]
		if i%2 == 0 {
			nibble >>= 4
		} else {
			nibble &= 0x0f
		}
		if nibble >= 8 {
			out[2+i] = c - 32 // minuscola -> maiuscola
		}
	}
	return string(out)
}

// IsValidAddress verifica forma e, se presente, checksum.
//
// Un indirizzo tutto minuscolo o tutto maiuscolo non porta checksum: per
// retrocompatibilita' EIP-55 lo considera valido. Un indirizzo misto invece
// DEVE avere il checksum corretto, altrimenti e' un errore di battitura.
func IsValidAddress(s string) bool {
	if !strings.HasPrefix(s, "0x") || len(s) != 2+AddressLength*2 {
		return false
	}
	body := s[2:]
	b, err := hex.DecodeString(body)
	if err != nil {
		return false
	}
	if body == strings.ToLower(body) || body == strings.ToUpper(body) {
		return true
	}
	return ToChecksumAddress(b) == s
}

// Sign firma un digest a 32 byte e restituisce 65 byte nell'ordine di Ethereum:
// R (32) ‖ S (32) ‖ V (1, valore 0 o 1).
//
// decred produce il formato compatto [V‖R‖S] con V in 27..30; Ethereum usa
// [R‖S‖V]. La conversione e' qui, in un posto solo.
func (w *Wallet) Sign(hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash da firmare: attesi 32 byte, ricevuti %d", len(hash))
	}
	sig := secpecdsa.SignCompact(w.priv, hash, false) // false = pubkey non compressa
	out := make([]byte, 65)
	copy(out[0:64], sig[1:65])
	out[64] = sig[0] - 27
	return out, nil
}

// RecoverAddress ricava l'indirizzo del firmatario da hash e firma.
//
// E' il pezzo che sblocca il punto 2 della roadmap: oggi VerifyTransaction e'
// inutilizzabile perche' serve la chiave pubblica del mittente, ma la
// transazione porta solo Sender, che ne e' l'hash — irreversibile. Con le firme
// recuperabili la pubkey si ricava dalla firma stessa: non serve trasportarla,
// e Sender torna verificabile confrontandolo con l'indirizzo recuperato.
func RecoverAddress(hash, sig []byte) (string, error) {
	if len(hash) != 32 {
		return "", fmt.Errorf("hash: attesi 32 byte, ricevuti %d", len(hash))
	}
	if len(sig) != 65 {
		return "", fmt.Errorf("firma: attesi 65 byte, ricevuti %d", len(sig))
	}
	if sig[64] > 3 {
		return "", fmt.Errorf("recovery id non valido: %d", sig[64])
	}
	dec := make([]byte, 65)
	dec[0] = sig[64] + 27
	copy(dec[1:], sig[0:64])
	pub, _, err := secpecdsa.RecoverCompact(dec, hash)
	if err != nil {
		return "", fmt.Errorf("recupero pubkey: %w", err)
	}
	raw := pub.SerializeUncompressed()
	return ToChecksumAddress(Keccak256(raw[1:])[12:]), nil
}

// CanonicalPublicKeyHex parses a secp256k1 public key (compressed or
// uncompressed), returns the canonical compressed hex representation and its
// EVM/HashBurst address. It is used for validator consensus keys, which are
// intentionally separate from reward and node/operator wallets.
func CanonicalPublicKeyHex(s string) (string, string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", "", fmt.Errorf("public key is not hex: %w", err)
	}
	pub, err := secp256k1.ParsePubKey(b)
	if err != nil {
		return "", "", fmt.Errorf("invalid secp256k1 public key: %w", err)
	}
	compressed := pub.SerializeCompressed()
	uncompressed := pub.SerializeUncompressed()
	addr := ToChecksumAddress(Keccak256(uncompressed[1:])[12:])
	return hex.EncodeToString(compressed), addr, nil
}

func (w *Wallet) PublicKeyHexCompressed() string {
	return hex.EncodeToString(w.priv.PubKey().SerializeCompressed())
}
