// hvm-wallet-init creates separate native HBT accounts locally. No network calls.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"strings"
)

type identity struct {
	Network           string `json:"network"`
	ChainID           uint64 `json:"chain_id"`
	Address           string `json:"address"`
	AssetType         string `json:"asset_type"`
	AllocationCreated bool   `json:"allocation_created"`
}

func exclusive(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}
func privateDirectory(path string) error {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return os.Mkdir(path, 0700)
	}
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("directory must be private, regular and not a symlink: %s", path)
	}
	return nil
}
func create(base, network string) (identity, error) {
	var id identity
	chain, ok := map[string]uint64{"testnet": 4735490, "mainnet": 4735489}[network]
	if !ok {
		return id, fmt.Errorf("choose testnet or mainnet")
	}
	if e := privateDirectory(base); e != nil {
		return id, e
	}
	dir := filepath.Join(base, fmt.Sprintf("%s-%d", network, chain))
	if e := privateDirectory(dir); e != nil {
		return id, e
	}
	key := filepath.Join(dir, "wallet.key")
	info, e := os.Lstat(key)
	if os.IsNotExist(e) {
		// Never replace a missing key when an existing public identity refers to it.
		if _, err := os.Lstat(filepath.Join(dir, "public.json")); !os.IsNotExist(err) {
			return id, fmt.Errorf("public identity exists but private key is missing; retained")
		}
		w, err := wallet.NewWallet()
		if err != nil {
			return id, err
		}
		if err = exclusive(key, []byte(hex.EncodeToString(w.PrivateKeyBytes())+"\n")); err != nil {
			return id, err
		}
		info, e = os.Lstat(key)
	}
	if e != nil {
		return id, e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return id, fmt.Errorf("private key must be regular and owner-only")
	}
	raw, e := os.ReadFile(key)
	if e != nil {
		return id, e
	}
	w, e := wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw)))
	if e != nil {
		return id, fmt.Errorf("invalid local private key; retained")
	}
	id = identity{network, chain, w.Address(), "native_coin", false}
	// Accidental reuse across the two local profiles is refused.
	other := "mainnet-4735489"
	if network == "mainnet" {
		other = "testnet-4735490"
	}
	if b, err := os.ReadFile(filepath.Join(base, other, "wallet.key")); err == nil {
		ow, err := wallet.FromPrivateKeyHex(strings.TrimSpace(string(b)))
		if err != nil {
			return id, fmt.Errorf("other network wallet key invalid")
		}
		if strings.EqualFold(ow.Address(), id.Address) {
			return id, fmt.Errorf("cross-network key reuse refused")
		}
	} else if !os.IsNotExist(err) {
		return id, err
	}
	public := filepath.Join(dir, "public.json")
	if info, err := os.Lstat(public); err == nil {
		if !info.Mode().IsRegular() {
			return id, fmt.Errorf("public identity must be a regular file")
		}
		b, err := os.ReadFile(public)
		if err != nil {
			return id, err
		}
		var saved identity
		if json.Unmarshal(b, &saved) != nil || saved != id {
			return id, fmt.Errorf("public identity mismatch; retained")
		}
	} else if os.IsNotExist(err) {
		b, _ := json.MarshalIndent(id, "", "  ")
		if e = exclusive(public, append(b, '\n')); e != nil {
			return id, e
		}
	} else {
		return id, err
	}
	d, e := os.Open(dir)
	if e != nil {
		return id, e
	}
	defer d.Close()
	if e = d.Sync(); e != nil {
		return id, e
	}
	return id, nil
}
func main() {
	network := flag.String("network", "", "testnet or mainnet")
	base := flag.String("directory", "/root/hashburst-native-wallets", "existing parent; base created with mode 0700")
	flag.Parse()
	id, e := create(*base, *network)
	if e != nil {
		fmt.Fprintln(os.Stderr, "STOP:", e)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(id)
}
