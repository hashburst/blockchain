package diagnostics

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnergyWrapAndBounds(t *testing.T) {
	for _, tt := range []struct {
		a, b, r              uint64
		seconds, watts, want float64
		bad                  bool
	}{
		{100, 300, 10000000, 1, 1, .0002, false}, {9999900, 100, 10000000, 1, 1, .0002, false},
		{100, 300, 10000000, 20, 1, 0, true}, {100, 300, 10000000, 1, math.NaN(), 0, true},
		{0, 2000000, 10000000, 1, 1, 0, true}, {10, 100, 100, 1, 1, 0, true},
	} {
		got, e := energyDelta(tt.a, tt.b, tt.r, time.Duration(tt.seconds*float64(time.Second)), tt.watts)
		if (e != nil) != tt.bad || (!tt.bad && got != tt.want) {
			t.Fatalf("%+v got %v %v", tt, got, e)
		}
	}
}
func TestPowercapAbsentAndRange(t *testing.T) {
	d := t.TempDir()
	if _, e := OpenPowercap(d); e == nil {
		t.Fatal("missing hardware accepted")
	}
	os.WriteFile(filepath.Join(d, "max_energy_range_uj"), []byte("10000\n"), 0600)
	os.WriteFile(filepath.Join(d, "energy_uj"), []byte("15\n"), 0600)
	c, e := OpenPowercap(d)
	if e != nil || c.RangeMicrojoules() != 10000 {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(d, "energy_uj"), []byte("10000"), 0600)
	if _, e = c.ReadMicrojoules(); e == nil {
		t.Fatal("range accepted")
	}
}

type fakeEnergy struct{ reads int }

func (c *fakeEnergy) ReadMicrojoules() (uint64, error) { c.reads++; return 0, nil }
func (c *fakeEnergy) RangeMicrojoules() uint64         { return 1000000000000 }
func TestEnergyBatchDoesNotReadInsideLoop(t *testing.T) {
	c := &fakeEnergy{}
	calls := 0
	r, e := MeasureEnergyBatch(c, 100, 1000, func() error {
		calls++
		if c.reads != 1 {
			t.Fatal("hot loop counter read")
		}
		return nil
	})
	if e != nil || calls != 100 || c.reads != 2 || r.Operations != 100 {
		t.Fatal(r, e, c, calls)
	}
	sentinel := errors.New("work failed")
	if _, e := MeasureEnergyBatch(c, 1, 1000, func() error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
}
