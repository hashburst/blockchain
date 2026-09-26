package testnet

// An EVM activation changes consensus rules. This command is deliberately
// offline, append-only in its evidence, and cannot alter keys, chain ID or state.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

	"hashburst/blockchain"
)

const EVMActivationMargin = uint64(1000)

type migrationEvidence struct {
	Old        Config            `json:"old"`
	Next       Config            `json:"next"`
	ConfigPath string            `json:"config_path"`
	Height     int               `json:"height"`
	Journals   map[string]string `json:"journal_sha256"`
}

func diskPin(c Config) []byte {
	return []byte(c.Pin() + "\n" + c.NodeID + "\n" + c.PeerID + "\n" + c.ValidatorID + "\n")
}
func digestFile(p string) (string, error) {
	b, e := os.ReadFile(p)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func validateEVMTransition(old, next Config) error {
	if e := old.Validate(); e != nil {
		return e
	}
	if e := next.Validate(); e != nil {
		return e
	}
	if old.Protocol.ChainID != 4735490 || old.Protocol.EVM != nil || next.Protocol.EVM == nil {
		return fmt.Errorf("only first testnet EVM activation is supported")
	}
	stripped := next
	stripped.Protocol.EVM = nil
	if !reflect.DeepEqual(old, stripped) {
		return fmt.Errorf("migration may only add protocol.evm; identity, genesis, keys and other rules must remain unchanged")
	}
	return nil
}

// atomicReplace retains owner/mode; the old file must be regular, never a link.
func atomicReplace(path string, data []byte) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".evm-migration-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(st.Mode().Perm()); e != nil {
		return e
	}
	if stat, ok := st.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
		if e = f.Chown(int(stat.Uid), int(stat.Gid)); e != nil {
			return e
		}
	}
	if _, e = f.Write(data); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// MigrateEVM can resume the pin/config two-file transition after interruption.
// Runtime startup fails closed between those writes. Repeating with the SAME
// candidate resumes; a different candidate cannot overwrite the evidence.
func MigrateEVM(configPath, nextPath string) error {
	configPath, e := filepath.Abs(configPath)
	if e != nil {
		return e
	}
	current, e := Load(configPath)
	if e != nil {
		return e
	}
	next, e := Load(nextPath)
	if e != nil {
		return e
	}
	evidencePath := filepath.Join(current.DataDir, "evm-activation-intent.json")
	old := current
	var evidence migrationEvidence
	raw, e := os.ReadFile(evidencePath)
	existing := e == nil
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if existing {
		st, e := os.Lstat(evidencePath)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("invalid migration evidence")
		}
		if e = json.Unmarshal(raw, &evidence); e != nil {
			return e
		}
		if evidence.ConfigPath != configPath || !reflect.DeepEqual(evidence.Next, next) {
			return fmt.Errorf("candidate differs from recorded migration")
		}
		old = evidence.Old
		if !reflect.DeepEqual(current, old) && !reflect.DeepEqual(current, next) {
			return fmt.Errorf("configuration changed outside migration")
		}
	}
	if e = validateEVMTransition(old, next); e != nil {
		return e
	}
	pinPath := filepath.Join(current.DataDir, "runtime.pin")
	pin, e := os.ReadFile(pinPath)
	if e != nil {
		return e
	}
	active := old
	if bytes.Equal(pin, diskPin(next)) && existing {
		active = next
	} else if !bytes.Equal(pin, diskPin(old)) {
		return fmt.Errorf("unexpected identity/configuration pin")
	}
	// Prepare takes the runtime's nonblocking exclusive lock and verifies keys,
	// journals, canonical history and validator membership before any mutation.
	state, e := Prepare(active, false)
	if e != nil {
		return e
	}
	defer state.Close()
	if reflect.DeepEqual(current, next) && bytes.Equal(pin, diskPin(next)) {
		return nil
	}
	height := state.Chain.Height()
	if uint64(height)+EVMActivationMargin >= next.Protocol.EVM.ActivationHeight {
		return fmt.Errorf("activation must be more than %d blocks ahead of local height %d", EVMActivationMargin, height)
	}
	if _, e = blockchain.OpenExistingBlockchain(next.DataDir, next.Protocol, next.GenesisHash, next.CheckpointHeight, next.CheckpointHash); e != nil {
		return fmt.Errorf("candidate replay: %w", e)
	}
	journals := map[string]string{}
	for _, name := range []string{"consensus-votes.jsonl", "consensus-bft-signatures.jsonl"} {
		h, e := digestFile(filepath.Join(next.DataDir, name))
		if e != nil {
			return e
		}
		journals[name] = h
	}
	if existing {
		if !reflect.DeepEqual(journals, evidence.Journals) {
			return fmt.Errorf("journal changed during incomplete migration")
		}
	} else {
		evidence = migrationEvidence{old, next, configPath, height, journals}
		raw, e = json.MarshalIndent(evidence, "", "  ")
		if e != nil {
			return e
		}
		if e = writeExclusive(evidencePath, append(raw, '\n')); e != nil {
			return e
		}
	}
	if !bytes.Equal(pin, diskPin(next)) {
		if e = atomicReplace(pinPath, diskPin(next)); e != nil {
			return e
		}
	}
	raw, e = json.MarshalIndent(next, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicReplace(configPath, append(raw, '\n')); e != nil {
		return e
	}
	for name, want := range journals {
		got, e := digestFile(filepath.Join(next.DataDir, name))
		if e != nil {
			return e
		}
		if got != want {
			return fmt.Errorf("journal preservation failed")
		}
	}
	return nil
}
