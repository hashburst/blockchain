package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestLoadOrCreateWritesCanonicalBase64AndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p2p.key")
	first, err := loadOrCreate(path)
	if err != nil { t.Fatal(err) }
	st, err := os.Stat(path)
	if err != nil { t.Fatal(err) }
	if st.Mode().Perm() != 0600 { t.Fatalf("mode=%#o want 0600", st.Mode().Perm()) }
	onDisk, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(onDisk)))
	if err != nil { t.Fatalf("key is not canonical base64: %v", err) }
	decoded, err := crypto.UnmarshalPrivateKey(raw)
	if err != nil { t.Fatalf("decoded key is not a libp2p private key: %v", err) }
	want, _ := peer.IDFromPrivateKey(first)
	got, _ := peer.IDFromPrivateKey(decoded)
	if got != want { t.Fatalf("decoded peer=%s want %s", got, want) }
	reloaded, err := loadOrCreate(path)
	if err != nil { t.Fatal(err) }
	again, _ := peer.IDFromPrivateKey(reloaded)
	if again != want { t.Fatalf("reloaded peer=%s want %s", again, want) }
}

func TestLoadOrCreateRejectsRawLibp2pKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p2p.key")
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil { t.Fatal(err) }
	raw, err := crypto.MarshalPrivateKey(key)
	if err != nil { t.Fatal(err) }
	if err = os.WriteFile(path, raw, 0600); err != nil { t.Fatal(err) }
	if _, err = loadOrCreate(path); err == nil { t.Fatal("raw non-base64 key was admitted") }
}
