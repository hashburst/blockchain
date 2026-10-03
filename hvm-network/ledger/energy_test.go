package ledger

import (
	"encoding/json"
	"hashburst/internal/diagnostics"
	"os"
	"strconv"
	"testing"
)

// Explicit opt-in experiment, not a CI energy assertion. Never run with -race,
// -cpuprofile, -memprofile, -trace or a concurrently active miner/profiler.
func TestEnergyFrameBatch(t *testing.T) {
	zone := os.Getenv("HB_ENERGY_ZONE")
	if zone == "" {
		t.Skip("hardware-domain energy experiment not requested")
	}
	n, err := strconv.Atoi(os.Getenv("HB_ENERGY_ITERATIONS"))
	if err != nil || n <= 0 || n > 1000000000 {
		t.Fatal("set HB_ENERGY_ITERATIONS in [1,1e9]")
	}
	watts, err := strconv.ParseFloat(os.Getenv("HB_ENERGY_MAX_WATTS"), 64)
	if err != nil {
		t.Fatal("set verified physical domain upper bound HB_ENERGY_MAX_WATTS (not TDP)")
	}
	counter, err := diagnostics.OpenPowercap(zone)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := AppendFrame(nil, make([]byte, 4096))
	if err != nil {
		t.Fatal(err)
	}
	work := func() error { _, e := FramePayload(frame); return e }
	for i := 0; i < 1000; i++ {
		if err = work(); err != nil {
			t.Fatal(err)
		}
	}
	result, err := diagnostics.MeasureEnergyBatch(counter, n, watts, work)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	t.Logf("HARDWARE_DOMAIN_ONLY zone=%s result=%s", zone, raw)
}
