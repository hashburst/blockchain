# HashBurst Adaptive Resource Orchestrator v1

Status: dry-run candidate. No resource mutation is implemented.

The orchestrator makes local mining-budget recommendations while HVM/TEP are protected workloads. Mining is elastic and subordinate to validator/network health. Consensus difficulty, APoW target/reward, transaction validity and validator voting rules are outside the controller's authority.

## v1 policy

For a 24-CPU validator the normal envelope reserves 8 CPUs and permits 16 for mining. At load >= 0.85 per CPU the recommendation falls by 4 CPUs; at load >= 1.10 per CPU it falls by 8. A protected service that is not active fails closed and emits no recommendation. Future versions may add pressure, latency, rejected-share and reward/energy efficiency signals.

Every output contains observed inputs, protected capacity, proposed CPU quota, reason codes, policy version and a decision SHA-256. Actuation is always none in v1 dry-run.

## Energy and environmental accounting

VPS CPU utilization is a workload proxy, not an electricity meter. The v1 decision therefore emits measurement_quality=proxy, joules=null, co2e_g=null, and claim=not_measured. A later adapter may accept RAPL, BMC, hypervisor/provider metering or an external meter with source/provenance and measurement boundaries. Carbon-intensity data must likewise carry source, geography/time interval and methodology before CO2e is computed. A telemetry/accounting record is not by itself a certified carbon credit.

## Expansion

The policy input/output contract is node-agnostic so the same controller can later include six additional VPS, standby validators, HashBurst full nodes, and physical/Neurallity cluster-miners. Actuation will be a separate reviewed gate using cgroup/systemd controls with rollback; it is intentionally absent here.
