package wallet

// keystore.go — Web3 Secret Storage Definition v3.
//
// E' lo stesso formato di geth e MetaMask: un file JSON con la chiave privata
// cifrata con una password. Implementarlo esattamente significa che il wallet di
// un utente HashBurst e' importabile in MetaMask, e viceversa. Un formato
// proprietario avrebbe funzionato uguale e non sarebbe servito a niente.
//
// SCHEMA
//   derivedKey = scrypt(password, salt, N, r, p, 32)
//   encKey     = derivedKey[0:16]
//   ciphertext = AES-128-CTR(encKey, iv, privateKey)
//   mac        = keccak256(derivedKey[16:32] ‖ ciphertext)
//
// Il MAC copre la CHIAVE DERIVATA e il ciphertext, non la password: verificarlo
// dice se la password e' giusta senza esporla, e intercetta un file manomesso.
// Va confrontato a tempo costante.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"
)

// Parametri scrypt "standard" di geth: ~1s e ~256 MB di RAM per derivazione.
// Sono lenti apposta: e' il costo che paga anche chi prova password a forza
// bruta sul file rubato.
const (
	StandardScryptN = 262144
	StandardScryptR = 8
	StandardScryptP = 1

	// Parametri "light": ~100ms. Per i test. Non usarli per chiavi vere.
	LightScryptN = 4096
	LightScryptR = 8
	LightScryptP = 1

	scryptDKLen = 32
)

type cipherparamsJSON struct {
	IV string `json:"iv"`
}

type cryptoJSON struct {
	Cipher       string                 `json:"cipher"`
	CipherText   string                 `json:"ciphertext"`
	CipherParams cipherparamsJSON       `json:"cipherparams"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

type keystoreV3 struct {
	Address string     `json:"address"`
	Crypto  cryptoJSON `json:"crypto"`
	ID      string     `json:"id"`
	Version int        `json:"version"`
}

// EncryptV3 cifra la chiave privata nel formato V3.
func (w *Wallet) EncryptV3(password string, scryptN, scryptR, scryptP int) ([]byte, error) {
	if password == "" {
		return nil, errors.New("password vuota: il keystore sarebbe cifrato con nulla")
	}

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}
	derived, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return nil, fmt.Errorf("scrypt: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("iv: %w", err)
	}
	block, err := aes.NewCipher(derived[:16])
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	priv := w.PrivateKeyBytes()
	ciphertext := make([]byte, len(priv))
	cipher.NewCTR(block, iv).XORKeyStream(ciphertext, priv)

	mac := Keccak256(derived[16:32], ciphertext)

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("uuid: %w", err)
	}

	ks := keystoreV3{
		// geth vuole l'indirizzo in minuscolo e senza 0x.
		Address: hex.EncodeToString(w.AddressBytes()),
		Crypto: cryptoJSON{
			Cipher:       "aes-128-ctr",
			CipherText:   hex.EncodeToString(ciphertext),
			CipherParams: cipherparamsJSON{IV: hex.EncodeToString(iv)},
			KDF:          "scrypt",
			KDFParams: map[string]interface{}{
				"dklen": scryptDKLen,
				"n":     scryptN,
				"p":     scryptP,
				"r":     scryptR,
				"salt":  hex.EncodeToString(salt),
			},
			MAC: hex.EncodeToString(mac),
		},
		ID:      id.String(),
		Version: 3,
	}
	return json.MarshalIndent(ks, "", "  ")
}

// DecryptV3 apre un keystore V3.
func DecryptV3(data []byte, password string) (*Wallet, error) {
	var ks keystoreV3
	if err := json.Unmarshal(data, &ks); err != nil {
		return nil, fmt.Errorf("keystore non è JSON valido: %w", err)
	}
	if ks.Version != 3 {
		return nil, fmt.Errorf("versione keystore %d non supportata (attesa 3)", ks.Version)
	}
	if ks.Crypto.Cipher != "aes-128-ctr" {
		return nil, fmt.Errorf("cifrario %q non supportato", ks.Crypto.Cipher)
	}
	if strings.ToLower(ks.Crypto.KDF) != "scrypt" {
		return nil, fmt.Errorf("kdf %q non supportata (solo scrypt)", ks.Crypto.KDF)
	}

	getInt := func(key string) (int, error) {
		v, ok := ks.Crypto.KDFParams[key]
		if !ok {
			return 0, fmt.Errorf("kdfparams: manca %q", key)
		}
		f, ok := v.(float64) // encoding/json decodifica i numeri come float64
		if !ok {
			return 0, fmt.Errorf("kdfparams.%s non è un numero", key)
		}
		return int(f), nil
	}
	n, err := getInt("n")
	if err != nil {
		return nil, err
	}
	r, err := getInt("r")
	if err != nil {
		return nil, err
	}
	p, err := getInt("p")
	if err != nil {
		return nil, err
	}
	dklen, err := getInt("dklen")
	if err != nil {
		return nil, err
	}

	saltStr, _ := ks.Crypto.KDFParams["salt"].(string)
	salt, err := hex.DecodeString(saltStr)
	if err != nil {
		return nil, fmt.Errorf("salt non esadecimale: %w", err)
	}
	iv, err := hex.DecodeString(ks.Crypto.CipherParams.IV)
	if err != nil {
		return nil, fmt.Errorf("iv non esadecimale: %w", err)
	}
	ciphertext, err := hex.DecodeString(ks.Crypto.CipherText)
	if err != nil {
		return nil, fmt.Errorf("ciphertext non esadecimale: %w", err)
	}
	wantMAC, err := hex.DecodeString(ks.Crypto.MAC)
	if err != nil {
		return nil, fmt.Errorf("mac non esadecimale: %w", err)
	}

	derived, err := scrypt.Key([]byte(password), salt, n, r, p, dklen)
	if err != nil {
		return nil, fmt.Errorf("scrypt: %w", err)
	}

	// Confronto a tempo costante: un confronto normale perde informazione sul
	// MAC atteso attraverso il tempo impiegato a fallire.
	if subtle.ConstantTimeCompare(Keccak256(derived[16:32], ciphertext), wantMAC) != 1 {
		return nil, errors.New("password errata o keystore manomesso")
	}

	block, err := aes.NewCipher(derived[:16])
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	priv := make([]byte, len(ciphertext))
	cipher.NewCTR(block, iv).XORKeyStream(priv, ciphertext)

	w, err := FromPrivateKeyBytes(priv)
	if err != nil {
		return nil, err
	}

	// La chiave decifrata deve produrre l'indirizzo dichiarato nel file.
	// Se non combacia, il keystore mente su cosa contiene.
	if ks.Address != "" {
		want := strings.TrimPrefix(strings.ToLower(ks.Address), "0x")
		if got := hex.EncodeToString(w.AddressBytes()); got != want {
			return nil, fmt.Errorf("il keystore dichiara 0x%s ma la chiave produce 0x%s", want, got)
		}
	}
	return w, nil
}

// SaveKeystore scrive il keystore su disco con permessi 0600.
// Il nome file segue la convenzione geth, cosi' la directory e' utilizzabile
// direttamente come keystore di geth.
func (w *Wallet) SaveKeystore(dir, password string, scryptN, scryptR, scryptP int) (string, error) {
	data, err := w.EncryptV3(password, scryptN, scryptR, scryptP)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creazione %s: %w", dir, err)
	}
	name := fmt.Sprintf("UTC--%s--%s",
		time.Now().UTC().Format("2006-01-02T15-04-05.000000000Z"),
		hex.EncodeToString(w.AddressBytes()))
	path := filepath.Join(dir, name)

	// O_EXCL: se il file esiste, non sovrascriviamo una chiave.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("apertura %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("scrittura %s: %w", path, err)
	}
	return path, nil
}

// LoadKeystore apre un keystore da file.
func LoadKeystore(path, password string) (*Wallet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lettura %s: %w", path, err)
	}
	return DecryptV3(data, password)
}
