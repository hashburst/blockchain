package wallet

import (
	"encoding/json"
	"testing"
)

func TestKeystoreRejectsMalformedKDFAndLengths(t *testing.T) {
	w, _ := NewWallet()
	raw, e := w.EncryptV3("test-password-only", LightScryptN, LightScryptR, LightScryptP)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = DecryptV3(raw, "test-password-only"); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*keystoreV3){
		func(k *keystoreV3) { k.Crypto.KDFParams["dklen"] = 1 }, func(k *keystoreV3) { k.Crypto.KDFParams["n"] = 262145 }, func(k *keystoreV3) { k.Crypto.KDFParams["n"] = 4096.5 }, func(k *keystoreV3) { k.Crypto.KDFParams["r"] = 100000 }, func(k *keystoreV3) { k.Crypto.CipherParams.IV = "00" }, func(k *keystoreV3) { k.Crypto.CipherText = "00" }, func(k *keystoreV3) { k.Crypto.MAC = "00" },
	} {
		var k keystoreV3
		json.Unmarshal(raw, &k)
		change(&k)
		b, _ := json.Marshal(k)
		if _, e = DecryptV3(b, "test-password-only"); e == nil {
			t.Fatal("invalid keystore accepted")
		}
	}
}
