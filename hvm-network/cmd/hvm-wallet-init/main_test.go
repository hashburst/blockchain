package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDistinctNetworksAndResume(t *testing.T) {
	base := filepath.Join(t.TempDir(), "wallets")
	a, e := create(base, "testnet")
	if e != nil {
		t.Fatal(e)
	}
	b, e := create(base, "mainnet")
	if e != nil {
		t.Fatal(e)
	}
	if a.Address == b.Address || a.ChainID == b.ChainID || a.AllocationCreated || b.AllocationCreated {
		t.Fatal("network separation/allocation")
	}
	again, e := create(base, "mainnet")
	if e != nil || again != b {
		t.Fatal("identity not preserved", e)
	}
	key := filepath.Join(base, "mainnet-4735489/wallet.key")
	st, _ := os.Stat(key)
	if st.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
	os.Remove(key)
	if _, e = create(base, "mainnet"); e == nil {
		t.Fatal("silently regenerated missing key")
	}
}
func TestNoSymlinkOrLegacy(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	os.Mkdir(target, 0700)
	link := filepath.Join(root, "link")
	os.Symlink(target, link)
	if _, e := create(link, "testnet"); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := create(filepath.Join(root, "new"), "legacy"); e == nil {
		t.Fatal("legacy accepted")
	}
}
