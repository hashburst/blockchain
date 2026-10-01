package diagnostics

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnergyCounter reports a hardware domain, never energy of a goroutine/process.
// A domain reset during a measurement invalidates the experiment externally.
type EnergyCounter interface {
	ReadMicrojoules() (uint64, error)
	RangeMicrojoules() uint64
}

type PowercapCounter struct {
	path string
	span uint64
}

// OpenPowercap opens an explicitly selected Linux powercap zone. Missing hardware
// or permission is an error: CPU time is never substituted for energy.
func OpenPowercap(zone string) (*PowercapCounter, error) {
	raw, err := os.ReadFile(filepath.Join(zone, "max_energy_range_uj"))
	if err != nil {
		return nil, err
	}
	span, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || span == 0 {
		return nil, errors.New("invalid powercap counter range")
	}
	c := &PowercapCounter{filepath.Join(zone, "energy_uj"), span}
	_, err = c.ReadMicrojoules()
	return c, err
}
func (c *PowercapCounter) RangeMicrojoules() uint64 { return c.span }
func (c *PowercapCounter) ReadMicrojoules() (uint64, error) {
	b, e := os.ReadFile(c.path)
	if e != nil {
		return 0, e
	}
	n, e := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if e != nil {
		return 0, e
	}
	if n >= c.span {
		return 0, errors.New("counter outside range")
	}
	return n, nil
}

type EnergyBatch struct {
	Operations               int     `json:"operations"`
	ElapsedSeconds           float64 `json:"elapsed_seconds"`
	DomainJoules             float64 `json:"domain_joules"`
	DomainJoulesPerOperation float64 `json:"domain_joules_per_operation"`
}

func energyDelta(before, after, span uint64, elapsed time.Duration, maxWatts float64) (float64, error) {
	if span == 0 || before >= span || after >= span || elapsed <= 0 || maxWatts <= 0 || math.IsNaN(maxWatts) || math.IsInf(maxWatts, 0) {
		return 0, errors.New("invalid energy measurement bounds")
	}
	// The physical upper bound MUST apply to the measured domain. Without it,
	// two reads cannot detect multiple wraps. Reject long/ambiguous batches.
	if elapsed.Seconds()*maxWatts >= float64(span)/1e6 {
		return 0, errors.New("batch too long to exclude multiple counter wraps")
	}
	var d uint64
	if after >= before {
		d = after - before
	} else {
		d = (span - before) + after
	}
	j := float64(d) / 1e6
	if j > elapsed.Seconds()*maxWatts {
		return 0, errors.New("counter jump exceeds physical bound")
	}
	return j, nil
}

// MeasureEnergyBatch makes only two counter reads outside the hot loop. Run on an
// isolated host without profilers/miners, after warmup, with matched idle baseline
// and repeated randomized A/B trials. maxWatts is a verified hardware upper bound,
// NOT TDP. Results include other activity in that domain and measurement overhead.
// This does not change affinity, service state, counters or power limits.
func MeasureEnergyBatch(c EnergyCounter, n int, maxWatts float64, fn func() error) (EnergyBatch, error) {
	var result EnergyBatch
	if n <= 0 || fn == nil || c == nil || maxWatts <= 0 || math.IsNaN(maxWatts) || math.IsInf(maxWatts, 0) {
		return result, errors.New("invalid experiment")
	}
	start := time.Now()
	before, e := c.ReadMicrojoules()
	if e != nil {
		return result, e
	}
	for i := 0; i < n; i++ {
		if e = fn(); e != nil {
			return result, fmt.Errorf("operation %d: %w", i, e)
		}
	}
	after, e := c.ReadMicrojoules()
	if e != nil {
		return result, e
	}
	elapsed := time.Since(start)
	j, e := energyDelta(before, after, c.RangeMicrojoules(), elapsed, maxWatts)
	if e != nil {
		return result, e
	}
	return EnergyBatch{n, elapsed.Seconds(), j, j / float64(n)}, nil
}
