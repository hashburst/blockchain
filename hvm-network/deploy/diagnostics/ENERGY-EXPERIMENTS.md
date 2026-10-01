# VPS measurement and energy experiments

These tools do not certify a release. The experimental ledger is not yet wired
into the live node's full history access paths. An OS sample of the previous
runtime cannot identify a regression in this new code.

## What to measure separately

1. Same production binary/config/data digest, replay mode, finalized height and
   transaction workload; record Go version, CPU, CPU affinity and governor.
2. Process CPU-seconds, all cgroup ancestor quotas, host steal and PSI, disk bytes,
   maximum RSS, Go heap live/alloc bytes and GC cycles. RSS is not the Go heap.
3. CPU pprof, heap/allocs, mutex/block and execution trace in separate runs of
   the same workload. Profile overhead is measured against an unprofiled run.
4. APoW in its own process/cgroup; validation and replay in another. Never infer
   mining from aggregate CPU `nice`, or use runtime.Gosched as a priority policy.
5. Baseline and candidate with identical verified state/receipt/journal outputs,
   cold versus warm cache explicitly distinguished. Report median, dispersion,
   sample count and P95 where enough samples exist. No Joule claims from CPU time.

The existing `internal/diagnostics/profile.go` opt-in profile flags apply only
when building that candidate. Do not restart a live validator solely to gather
profiles, or expose public pprof endpoints. Reproduce on a copy first.

## Optional Linux powercap experiment

`internal/diagnostics/energy.go` reads a selected hardware energy domain twice,
not once per operation. Missing counters or access rights return an error. No
power cap, service, affinity, signature key or blockchain state is changed.

Build with the repository Go version. On a dedicated, isolated physical host,
select a real powercap domain and a verified physical maximum wattage for that
same domain (TDP is not that bound). Then, from `hvm-network`:

```sh
# Set these values from the test host specification; no guessed defaults.
export HB_ENERGY_ZONE=/sys/class/powercap/intel-rapl:0
export HB_ENERGY_ITERATIONS=1000000
# export HB_ENERGY_MAX_WATTS=<verified physical domain upper bound>
go test ./ledger -run '^TestEnergyFrameBatch$' -count=1 -v
```

Do not use `-race`, profiling or trace in an energy trial. Without the opt-in
zone the test skips. With an unavailable zone it fails rather than inventing a
result. The counter measures the whole domain, not one function, process or VM.
Counter reads, the Go test harness, OS and unrelated activity add overhead.
There is no zero-overhead energy measurement. Counter resets invalidate a trial.
The implementation rejects intervals long enough for ambiguous multiple wraps,
conditional on the supplied physical wattage bound being true.

For function experiments use the same `MeasureEnergyBatch(counter, n, bound,
func() error { ... })` wrapper and realistic fixed fixtures. Warm up first,
validate outputs outside timing, run matched idle windows, randomize baseline /
candidate order (ABBA or randomized blocks), and repeat. Save raw domain Joules
and elapsed time; retain even negative baseline-subtracted estimates with their
uncertainty. Do not present such an estimate as exact per-function energy.
Measure APoW separately with explicit proof difficulty/throughput and identical
work. Tests here do not measure hardware energy in CI or on the submitted VPS.

## Analyze without truncating a report after a failed command

On the Mac, use the analyzer shipped in the package, not `/root/diagnostics` on
the VPS unless it was copied there too:

```sh
python3 diagnostics/analyze-samples.py v2-samples.jsonl > v2-analysis.tmp &&
  mv v2-analysis.tmp v2-analysis.json
```

Supply the actual path of the copied JSONL. `>` creates/truncates its target even
if Python subsequently reports a missing script. The input JSONL is retained.

## Sources

- https://go.dev/doc/diagnostics
- https://www.kernel.org/doc/html/latest/filesystems/proc.html
- https://www.kernel.org/doc/html/latest/accounting/psi.html
- https://www.kernel.org/doc/html/latest/power/powercap/powercap.html
