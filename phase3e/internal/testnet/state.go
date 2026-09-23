package testnet

import (
	"encoding/base64"
	"fmt"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sys/unix"
	"hashburst/blockchain"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"strings"
)

type State struct {
	Config Config
	Chain  *blockchain.Blockchain
	Key    crypto.PrivKey
	Signer *wallet.Wallet
	lock   *os.File
}

func (s *State) Close() {
	if s.lock != nil {
		_ = unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
		_ = s.lock.Close()
		s.lock = nil
	}
}
func privateFile(path string) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("key must be a private regular file")
	}
	return os.ReadFile(path)
}
func Prepare(c Config, provision bool) (out *State, err error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	real, e := filepath.EvalSymlinks(c.DataDir)
	if e != nil {
		return nil, e
	}
	if real != c.DataDir {
		return nil, fmt.Errorf("state path must be canonical, without symlinks")
	}
	st, e := os.Stat(real)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() || st.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("state directory must not be group/world writable")
	}
	fd, e := unix.Open(filepath.Join(real, "runtime.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	s := &State{Config: c, lock: os.NewFile(uintptr(fd), "runtime.lock")}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		return nil, fmt.Errorf("state already in use: %w", e)
	}
	raw, e := privateFile(c.P2PKeyFile)
	if e != nil {
		return nil, e
	}
	b, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil {
		return nil, e
	}
	s.Key, e = crypto.UnmarshalPrivateKey(b)
	if e != nil {
		return nil, e
	}
	id, e := peer.IDFromPrivateKey(s.Key)
	if e != nil {
		return nil, e
	}
	if id.String() != c.PeerID {
		return nil, fmt.Errorf("P2P key mismatch")
	}
	pin := c.Pin() + "\n" + c.NodeID + "\n" + c.PeerID + "\n" + c.ValidatorID + "\n"
	pinPath := filepath.Join(real, "runtime.pin")
	if !provision {
		st, e := os.Lstat(pinPath)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("invalid pin file")
		}
		b, e := os.ReadFile(pinPath)
		if e != nil {
			return nil, e
		}
		if string(b) != pin {
			return nil, fmt.Errorf("network/config/identity pin mismatch")
		}
	}
	for _, name := range []string{"consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		st, e := os.Lstat(filepath.Join(real, name))
		if os.IsNotExist(e) && provision {
			continue
		}
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("journal must be a regular file")
		}
	}
	s.Chain, e = blockchain.OpenExistingBlockchain(real, c.Protocol, c.GenesisHash, c.CheckpointHeight, c.CheckpointHash)
	if e != nil {
		return nil, e
	}
	if c.Role == "validator" {
		raw, e = privateFile(c.ConsensusKeyFile)
		if e != nil {
			return nil, e
		}
		s.Signer, e = wallet.FromPrivateKeyHex(strings.TrimSpace(string(raw)))
		if e != nil {
			return nil, e
		}
		v, ok := s.Chain.ValidatorRegistry().Get(c.ValidatorID)
		if !ok || !wallet.AddressEqual(v.ConsensusAddress, s.Signer.Address()) || v.PeerID != c.PeerID {
			return nil, fmt.Errorf("validator/key/peer registry mismatch")
		}
		if e = s.Chain.CheckValidatorRestart(c.ValidatorID); e != nil {
			return nil, e
		}
	}
	if provision {
		if s.Chain.Height() != c.CheckpointHeight {
			return nil, fmt.Errorf("provision requires exact prepared checkpoint height")
		}
		if _, e = os.Lstat(pinPath); !os.IsNotExist(e) {
			return nil, fmt.Errorf("pin already exists or cannot be inspected")
		}
		for _, name := range []string{"consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
			p := filepath.Join(real, name)
			if _, e = os.Stat(p); os.IsNotExist(e) {
				if e = writeExclusive(p, nil); e != nil {
					return nil, e
				}
			}
		}
		if e = writeExclusive(pinPath, []byte(pin)); e != nil {
			return nil, e
		}
	}
	return s, nil
}
func writeExclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
