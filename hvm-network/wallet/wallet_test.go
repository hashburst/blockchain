package wallet

// wallet_test.go — verifica contro valori pubblici e noti, non contro se stessi.
//
// Un test che confronta l'output della funzione con l'output della funzione
// passa sempre, anche quando la funzione e' sbagliata. Qui si confronta con i
// vettori dell'EIP-55, con l'hash keccak della stringa vuota e con indirizzi
// derivati da chiavi private note a tutto l'ecosistema: se questi passano,
// l'implementazione e' compatibile con Ethereum, punto.

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// TestKeccakNonESHA3 e' il test piu' importante del file.
//
// Keccak-256 e SHA3-256 differiscono solo nel padding: producono output
// plausibili e completamente diversi. Usare sha3.New256() al posto di
// sha3.NewLegacyKeccak256() darebbe indirizzi ben formati, deterministici e
// incompatibili con qualunque wallet al mondo — e nulla lo segnalerebbe.
//
// c5d2460186f7233c... e' keccak256("") ed e' una costante nota di Ethereum.
func TestKeccakNonESHA3(t *testing.T) {
	const emptyKeccak256 = "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
	got := hex.EncodeToString(Keccak256([]byte{}))
	if got != emptyKeccak256 {
		t.Fatalf("Keccak256(\"\") = %s\nvoluto                = %s\n"+
			"Se il valore inizia con a7ffc6f8 stai usando SHA3-256, non Keccak-256.", got, emptyKeccak256)
	}
}

// TestEIP55Vettori usa i quattro esempi contenuti nell'EIP-55 stesso.
func TestEIP55Vettori(t *testing.T) {
	vettori := []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	}
	for _, want := range vettori {
		raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(want), "0x"))
		if err != nil {
			t.Fatalf("vettore non esadecimale: %v", err)
		}
		if got := ToChecksumAddress(raw); got != want {
			t.Errorf("checksum errato:\n  ottenuto %s\n  atteso   %s", got, want)
		}
		if !IsValidAddress(want) {
			t.Errorf("IsValidAddress rifiuta un indirizzo valido: %s", want)
		}
	}
}

// TestChecksumRilevaErroreDiBattitura: e' lo scopo dell'EIP-55.
func TestChecksumRilevaErroreDiBattitura(t *testing.T) {
	valido := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	// stessa stringa con una lettera di case sbagliato
	corrotto := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"
	if IsValidAddress(corrotto) {
		t.Error("il checksum non rileva la modifica: e' inutile")
	}
	if !IsValidAddress(valido) {
		t.Error("l'indirizzo valido viene rifiutato")
	}
}

// TestIndirizziDaChiaviNote: chiavi private 1, 2, 3 e i loro indirizzi, noti a
// chiunque abbia usato una testnet. Provano curva (secp256k1), hash (keccak) e
// troncamento (ultimi 20 byte) tutti insieme.
func TestIndirizziDaChiaviNote(t *testing.T) {
	casi := []struct{ key, addr string }{
		{"0000000000000000000000000000000000000000000000000000000000000001",
			"0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"},
		{"0000000000000000000000000000000000000000000000000000000000000002",
			"0x2b5ad5c4795c026514f8317c7a215e218dccd6cf"},
		{"0000000000000000000000000000000000000000000000000000000000000003",
			"0x6813eb9362372eef6200f3b1dbc3f819671cba69"},
	}
	for _, c := range casi {
		w, err := FromPrivateKeyHex(c.key)
		if err != nil {
			t.Fatalf("chiave %s: %v", c.key[:8], err)
		}
		// confronto senza case: il checksum e' gia' coperto da TestEIP55Vettori
		if got := strings.ToLower(w.Address()); got != c.addr {
			t.Errorf("chiave ...%s:\n  ottenuto %s\n  atteso   %s", c.key[56:], got, c.addr)
		}
	}
}

func TestIndirizzoLungo20Byte(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.AddressBytes()) != AddressLength {
		t.Fatalf("indirizzo di %d byte, attesi %d", len(w.AddressBytes()), AddressLength)
	}
	if len(w.Address()) != 42 {
		t.Fatalf("indirizzo testuale di %d caratteri, attesi 42", len(w.Address()))
	}
}

func TestFirmaERecupero(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	hash := Keccak256([]byte("transazione di prova"))

	sig, err := w.Sign(hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 65 {
		t.Fatalf("firma di %d byte, attesi 65", len(sig))
	}

	addr, err := RecoverAddress(hash, sig)
	if err != nil {
		t.Fatal(err)
	}
	if addr != w.Address() {
		t.Fatalf("indirizzo recuperato %s, atteso %s", addr, w.Address())
	}

	// Un hash diverso non deve recuperare lo stesso indirizzo.
	altro := Keccak256([]byte("un'altra transazione"))
	if a, err := RecoverAddress(altro, sig); err == nil && a == w.Address() {
		t.Fatal("la firma e' valida per un messaggio diverso: non vincola nulla")
	}
}

func TestKeystoreAndataERitorno(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	// parametri light: quelli standard richiedono ~1s e 256MB
	data, err := w.EncryptV3("password-di-prova", LightScryptN, LightScryptR, LightScryptP)
	if err != nil {
		t.Fatal(err)
	}

	// La chiave privata non deve comparire in chiaro nel file.
	if bytes.Contains(data, []byte(hex.EncodeToString(w.PrivateKeyBytes()))) {
		t.Fatal("la chiave privata e' in chiaro nel keystore")
	}

	back, err := DecryptV3(data, "password-di-prova")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back.PrivateKeyBytes(), w.PrivateKeyBytes()) {
		t.Fatal("la chiave decifrata non corrisponde")
	}
	if back.Address() != w.Address() {
		t.Fatalf("indirizzo %s, atteso %s", back.Address(), w.Address())
	}
}

func TestKeystorePasswordErrata(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	data, err := w.EncryptV3("giusta", LightScryptN, LightScryptR, LightScryptP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptV3(data, "sbagliata"); err == nil {
		t.Fatal("password errata accettata: il MAC non viene verificato")
	}
}

func TestKeystoreManomesso(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	data, err := w.EncryptV3("password", LightScryptN, LightScryptR, LightScryptP)
	if err != nil {
		t.Fatal(err)
	}
	// altera un carattere del ciphertext
	corrotto := bytes.Replace(data, []byte(`"ciphertext": "`), []byte(`"ciphertext": "ff`), 1)
	if _, err := DecryptV3(corrotto, "password"); err == nil {
		t.Fatal("keystore manomesso accettato: il MAC non protegge il ciphertext")
	}
}
