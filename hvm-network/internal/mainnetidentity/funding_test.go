package mainnetidentity

import (
	"encoding/hex"
	"encoding/json"
	"hashburst/wallet"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFundingConservationOrderingAndRejection(t *testing.T) {
	c := fixture()
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	var identities []Identity
	for i := 0; i < 4; i++ {
		dir := filepath.Join(parent, string(rune('a'+i)))
		o := Options{Config: c, NodeID: string(rune('a' + i)), IP: "127.0.0.1", P2PPort: 31317 + i, TEPPublicKey: strings.Repeat(string(rune('1'+i)), 64)}
		if e := Generate(dir, o); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(dir, "public.json"))
		if e != nil {
			t.Fatal(e)
		}
		var v Identity
		if e = json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		identities = append(identities, v)
	}
	p, e := PlanFunding(c, identities)
	if e != nil {
		t.Fatal(e)
	}
	if p.DebitedUnits != p.BondUnits+p.FeeUnits || p.RemainingFounderAllocation+p.DebitedUnits != c.GenesisImport.FounderUnits {
		t.Fatal("conservation")
	}
	for i, tx := range p.Transfers {
		if tx.Sequence != uint64(i) || tx.Signature != "" || tx.ChainID != 4735489 {
			t.Fatal("transfer envelope")
		}
	}
	if VerifyFunding(c, identities, p.Transfers) == nil {
		t.Fatal("unsigned transfers accepted")
	}
	w, e := wallet.NewWallet()
	if e != nil {
		t.Fatal(e)
	}
	for _, tx := range p.Transfers {
		sig, e := w.Sign(tx.SigningHash())
		if e != nil {
			t.Fatal(e)
		}
		tx.Signature = hex.EncodeToString(sig)
	}
	if VerifyFunding(c, identities, p.Transfers) == nil {
		t.Fatal("wrong founder accepted")
	}
	p, _ = PlanFunding(c, identities)
	identities[0], identities[3] = identities[3], identities[0]
	q, e := PlanFunding(c, identities)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(q)
	if string(a) != string(b) {
		t.Fatal("order changed plan")
	}
	q.Transfers[0].ValueUnits++
	if VerifyFunding(c, identities, q.Transfers) == nil {
		t.Fatal("modified amount accepted")
	}
	if VerifyFunding(c, identities, nil) == nil {
		t.Fatal("empty funding accepted")
	}
	identities[0] = identities[1]
	if _, e := PlanFunding(c, identities); e == nil {
		t.Fatal("duplicate accepted")
	}
}
