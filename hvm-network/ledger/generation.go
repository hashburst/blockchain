package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// GenerationProof describes byte-level conversion only; it is NOT a quorum
// certificate, mainnet genesis or proof that legacy spending is frozen.
type GenerationProof struct {
	Format      string `json:"format"`
	Count       uint64 `json:"count"`
	DataSHA256  string `json:"data_sha256"`
	IndexSHA256 string `json:"index_sha256"`
}

func syncDirectory(p string) error {
	d, e := os.Open(p)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// WriteGeneration writes only a NEW sibling directory. Source records are never
// changed. The caller exclusively owns the trusted parent and source snapshot.
// next returns a verified, encoded HBB2 block for the requested ordinal.
// The complete directory becomes visible atomically after data Sync, index Sync,
// manifest Sync and directory Sync; no separate dat/idx replacement is attempted.
// This is an OFFLINE conversion writer, not a live consensus/WAL/checkpoint path.
func WriteGeneration(destination string, count uint64, next func(uint64) ([]byte, error)) (proof GenerationProof, err error) {
	if count == 0 || next == nil || !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return proof, errors.New("new absolute destination and nonempty source required")
	}
	parent := filepath.Dir(destination)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil {
		return proof, e
	}
	if real != parent {
		return proof, errors.New("canonical parent required")
	}
	st, e := os.Stat(parent)
	if e != nil {
		return proof, e
	}
	if !st.IsDir() || st.Mode().Perm()&0022 != 0 {
		return proof, errors.New("parent must not be group/world writable")
	}
	// Cooperative writer exclusion; never steal or delete another writer's lock.
	lock, e := os.OpenFile(destination+".conversion-lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return proof, e
	}
	defer func() { lock.Close(); os.Remove(destination + ".conversion-lock") }()
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return proof, errors.New("destination exists or cannot be inspected")
	}
	tmp, e := os.MkdirTemp(parent, ".hvm-generation-")
	if e != nil {
		return proof, e
	}
	published := false
	defer func() {
		if !published {
			os.RemoveAll(tmp)
		}
	}()
	data, e := os.OpenFile(filepath.Join(tmp, "blockchain.dat"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return proof, e
	}
	defer data.Close()
	idx, e := os.OpenFile(filepath.Join(tmp, "blockchain.idx"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return proof, e
	}
	defer idx.Close()
	dh, ih := sha256.New(), sha256.New()
	dw, iw := io.MultiWriter(data, dh), io.MultiWriter(idx, ih)
	var offset uint64
	var frame []byte
	for n := uint64(0); n < count; n++ {
		payload, e := next(n)
		if e != nil {
			return proof, e
		}
		frame, e = AppendFrame(frame[:0], payload)
		if e != nil {
			return proof, e
		}
		if offset > uint64(1<<63-1)-uint64(len(frame)) {
			return proof, errors.New("data offset overflow")
		}
		if _, e = dw.Write(frame); e != nil {
			return proof, e
		}
		rec := IndexRecord{n, offset, uint32(len(frame))}.Encode()
		if _, e = iw.Write(rec[:]); e != nil {
			return proof, e
		}
		offset += uint64(len(frame))
	}
	// Index entries are not published to live readers before data becomes durable.
	if e = data.Sync(); e != nil {
		return proof, e
	}
	if e = idx.Sync(); e != nil {
		return proof, e
	}
	if e = data.Close(); e != nil {
		return proof, e
	}
	if e = idx.Close(); e != nil {
		return proof, e
	}
	proof = GenerationProof{"HBB2-in-HBX2-index20-BE", count, hex.EncodeToString(dh.Sum(nil)), hex.EncodeToString(ih.Sum(nil))}
	raw, e := json.MarshalIndent(proof, "", "  ")
	if e != nil {
		return proof, e
	}
	f, e := os.OpenFile(filepath.Join(tmp, "FORMAT.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return proof, e
	}
	_, e = f.Write(append(raw, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return proof, e
	}
	if ce != nil {
		return proof, ce
	}
	if e = syncDirectory(tmp); e != nil {
		return proof, e
	}
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return proof, errors.New("destination appeared during conversion")
	}
	if e = os.Rename(tmp, destination); e != nil {
		return proof, e
	}
	published = true
	// If this final sync fails, destination may exist: retain it, do NOT blindly retry.
	return proof, syncDirectory(parent)
}
