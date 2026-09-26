package hvm

import "fmt"

type Meter struct {
	Limit uint64
	Used  uint64
}

func NewMeter(limit uint64) *Meter { return &Meter{Limit: limit} }

func (m *Meter) Consume(units uint64) error {
	if units > ^uint64(0)-m.Used || m.Used+units > m.Limit {
		// Out-of-compute consumes the entire declared limit. This makes the
		// failure deterministic and prevents callers from probing expensive
		// paths while only paying for the prefix reached before the limit.
		m.Used = m.Limit
		return fmt.Errorf("compute limit exceeded: limit=%d", m.Limit)
	}
	m.Used += units
	return nil
}

const (
	ComputeBaseTransfer   uint64 = 1_000
	ComputeBaseDeploy     uint64 = 20_000
	ComputeBaseCall       uint64 = 5_000
	ComputeStateRead      uint64 = 50
	ComputeStateWrite     uint64 = 500
	ComputeEventBase      uint64 = 200
	ComputePerPayloadByte uint64 = 2
	ComputePayoutItem     uint64 = 1_500
	ComputeTransitionItem uint64 = 900
)
