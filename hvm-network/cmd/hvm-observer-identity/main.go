package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"os"
	"strings"
)

func loadOrCreate(path string) (crypto.PrivKey, error) {
	if raw, err := os.ReadFile(path); err == nil {
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("unsafe key file")
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, err
		}
		return crypto.UnmarshalPrivateKey(decoded)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if _, err = f.WriteString(base64.StdEncoding.EncodeToString(raw) + "\n"); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return key, nil
}

func main() {
	p := flag.String("key", "", "observer p2p private key")
	flag.Parse()
	if *p == "" {
		panic("--key required")
	}
	k, err := loadOrCreate(*p)
	if err != nil {
		panic(err)
	}
	id, err := peer.IDFromPrivateKey(k)
	if err != nil {
		panic(err)
	}
	fmt.Println(id.String())
}
