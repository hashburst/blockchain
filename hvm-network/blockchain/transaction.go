package blockchain

// transaction.go — transazioni con firme verificabili.
//
// CAMBIO RISPETTO ALLA VERSIONE PRECEDENTE
//
// Prima SignTransaction/VerifyTransaction usavano crypto/ecdsa su P-256, la
// curva del vecchio wallet. Non li chiamava nessuno — e non potevano essere
// chiamati: VerifyTransaction richiede la chiave pubblica del mittente, ma la
// transazione porta solo Sender, che ne e' l'hash (irreversibile).
//
// Ora la firma e' secp256k1 recuperabile (la stessa del wallet EVM): la chiave
// pubblica si ricava DALLA firma, quindi non serve trasportarla, e Sender torna
// verificabile confrontandolo con l'indirizzo recuperato. Questo e' il pezzo
// che rende reale il "modello di stato": senza firma verificabile, "il saldo di
// un indirizzo" non significa niente, perche' chiunque potrebbe spendere da
// qualsiasi indirizzo.

import (
	"fmt"

	"hashburst/wallet"
)

// Indirizzi speciali. Non sono chiavi: sono sentinelle riconosciute dal
// protocollo, e le transazioni che li usano come Sender NON richiedono firma.
const (
	SystemSender    = "System"                                       // reward di mining
	RegistryAddress = "0x0000000000000000000000000000000000REGISTRY" // NODE_REGISTRATION
)

// Transaction rappresenta un trasferimento o un messaggio applicativo.
type Transaction struct {
	ID        string
	Sender    string
	Receiver  string
	Amount    float64
	Nonce     int64
	Data      string
	Signature string // hex di 65 byte (R‖S‖V), vuoto per le tx di sistema
	PubKey    string // hex della pubkey non compressa; opzionale, la firma la recupera
}

// NewTransaction crea una transazione di trasferimento non firmata.
func NewTransaction(sender, receiver string, amount float64) *Transaction {
	t := &Transaction{
		Sender:   sender,
		Receiver: receiver,
		Amount:   amount,
		Nonce:    randomNonce(),
	}
	t.ID = t.HashTransaction()
	return t
}

// NewDataTransaction crea una transazione che porta un payload in Data.
// Usata da NewNodeRegistration: il record del nodo entra qui, dentro l'hash.
func NewDataTransaction(sender, receiver string, amount float64, data string) *Transaction {
	t := &Transaction{
		Sender:   sender,
		Receiver: receiver,
		Amount:   amount,
		Nonce:    randomNonce(),
		Data:     data,
	}
	t.ID = t.HashTransaction()
	return t
}

func randomNonce() int64 {
	return wallet.RandomInt63()
}

// signingHash e' cio' che viene firmato: la codifica canonica di TUTTI i campi
// che definiscono la transazione, tranne firma, pubkey e ID (che ne derivano).
// Include ChainID: una firma valida su questa chain non lo e' su un'altra.
func (t *Transaction) signingHash() []byte {
	return wallet.Keccak256(t.canonicalBytes())
}

func (t *Transaction) canonicalBytes() []byte {
	buf := newCanonicalBuffer()
	buf.putString(ChainID)
	buf.putString(t.Sender)
	buf.putString(t.Receiver)
	buf.putInt64(AmountToUnits(t.Amount))
	buf.putInt64(t.Nonce)
	buf.putString(t.Data)
	return buf.bytes()
}

// HashTransaction: l'ID della transazione, hex del keccak canonico.
// Nota: usa keccak, non piu' sha256. Coerente con lo schema EVM.
func (t *Transaction) HashTransaction() string {
	return fmt.Sprintf("%x", t.signingHash())
}

// Sign firma la transazione con il wallet del mittente e verifica che il
// wallet corrisponda davvero a Sender: firmare per conto di un altro indirizzo
// e' un errore del chiamante, non qualcosa da lasciar passare.
func (t *Transaction) Sign(w *wallet.Wallet) error {
	if t.Sender != w.Address() {
		return fmt.Errorf("il wallet %s non corrisponde al mittente %s", w.Address(), t.Sender)
	}
	sig, err := w.Sign(t.signingHash())
	if err != nil {
		return err
	}
	t.Signature = fmt.Sprintf("%x", sig)
	t.PubKey = fmt.Sprintf("%x", w.PublicKeyBytes())
	t.ID = t.HashTransaction()
	return nil
}

// IsSystem indica una transazione che il protocollo genera da se' (reward) e
// che quindi non porta firma.
func (t *Transaction) IsSystem() bool {
	return t.Sender == SystemSender
}

// Verify controlla che la transazione sia legittima.
//
//   - tx di sistema (reward): Sender == "System", nessuna firma. Le regole sul
//     loro contenuto (importo == reward, una sola per blocco) sono in
//     ValidateBlockAgainst, non qui.
//   - NODE_REGISTRATION: firmata come una normale tx dal wallet del nodo.
//   - tx normali: la firma deve recuperare esattamente Sender.
//
// E' il punto in cui "Sender" smette di essere una stringa che ci si fida e
// diventa un fatto crittografico.
func (t *Transaction) Verify() error {
	if t.IsSystem() {
		return nil // la validita' di una reward si giudica nel contesto del blocco
	}
	if t.ID != t.HashTransaction() {
		return fmt.Errorf("ID non corrisponde al contenuto")
	}
	if !wallet.IsValidAddress(t.Sender) {
		return fmt.Errorf("mittente %q non e' un indirizzo valido", t.Sender)
	}
	if t.Receiver != RegistryAddress && !wallet.IsValidAddress(t.Receiver) {
		return fmt.Errorf("destinatario %q non e' un indirizzo valido", t.Receiver)
	}
	if AmountToUnits(t.Amount) < 0 {
		return fmt.Errorf("importo negativo")
	}
	sig, err := hexTo65(t.Signature)
	if err != nil {
		return fmt.Errorf("firma: %w", err)
	}
	recovered, err := wallet.RecoverAddress(t.signingHash(), sig)
	if err != nil {
		return fmt.Errorf("recupero firmatario: %w", err)
	}
	if !wallet.AddressEqual(recovered, t.Sender) {
		return fmt.Errorf("firma di %s, ma Sender dichiara %s", recovered, t.Sender)
	}
	return nil
}

// IsWellFormed: controllo strutturale rapido usato in validazione blocco.
// Le tx di sistema sono ben formate per definizione (le loro regole sono
// altrove); per tutte le altre, ben formata significa firma valida.
func (t *Transaction) IsWellFormed() bool {
	if t.IsSystem() {
		return t.ID == t.HashTransaction()
	}
	return t.Verify() == nil
}

func hexTo65(s string) ([]byte, error) {
	b, err := wallet.HexToBytes(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 65 {
		return nil, fmt.Errorf("attesi 65 byte, ricevuti %d", len(b))
	}
	return b, nil
}
