# VPS v2 observations, 2026-10-01 UTC

Two distinct 12-sample windows were supplied. They are not a single continuous
record. The standalone JSONL matches the submitted summary; the pasted terminal
Markdown contains a later capture. Both identify PID 1270573, 26 threads,
`ActiveState=active`, `NRestarts=0`.

| Metric | 22:12:21-22:13:21 UTC | 22:24:13-22:25:13 UTC |
|---|---:|---:|
| Monotonic duration | 59.994608 s | 60.092887 s |
| Process CPU | 0.73 CPU-s | 13.57 CPU-s |
| Average percentage of ONE core | 1.2168% | 22.5817% |
| Host aggregate steal | 85.4603% | 85.1915% |
| Service cgroup throttled time delta | 0 | 0 |
| Service cpu.max | max 100000 | max 100000 |
| Process status RSS | 881180 KiB, constant | 881180 KiB, constant |
| smaps RSS change | 0 KiB | +8 KiB |
| Physical process read bytes | 0 | 0 |
| Physical process write bytes | 0 | 32768 |
| CPU PSI some, delta / duration | 0.7902% | 1.8459% |
| I/O PSI some, delta / duration | 0% | 0.03265% |
| Memory PSI delta | 0 | 0 |
| Energy | unmeasured | unmeasured |

RSS from `/proc/status` is 860.53 MiB. It is not Alloc, HeapAlloc or allocation
rate. Its small discrepancy with smaps is not a leak measurement. No major
faults, swap or OOM events are present in the captures. The second window has
three additional minor faults. rchar/wchar are not physical disk traffic.

Steal is calculated from the delta of the first eight aggregate `/proc/stat`
CPU fields, excluding guest/guest_nice to avoid double counting. It is time
virtual CPUs were unavailable to this guest, not a measurement of this node's
PoH execution. Service-level unlimited quota and zero throttling do not exclude
parent cgroup or hypervisor/provider limits. Escalate both windows to the VPS
provider and request investigation of CPU scheduling/oversubscription/limits.
Do not assume a specific provider cause or an identical cause for an earlier
replay from these observations alone. High guest nice time cannot identify APoW.

CPU PSI is wall time with runnable workload delayed; CPU-seconds and PSI overlap
in scope and cannot be subtracted to produce "pure PoH". A leader-only schedstat
or blocked-I/O counter also does not describe all Go threads.

These windows show no substantial I/O pressure or RSS growth, but do not exclude
longer-term leaks, deadlocks, rare failures or expensive cold replay. They lack
function profiles, Go heap metrics, binary identity, input/workload and finality
progress. They cannot certify ledger v0.7.0-dev.1, which was distributed as
experimental source and tools, not installed into the running node.

## Measured component improvement

The checkpoint reader now allocates the bounded encoded file and authenticated
expanded payload once each, replacing geometric ReadAll growth. HMAC precedes
expansion; limits, gzip CRC/trailer, exact length and truncation checks remain.
This changes neither the file format nor consensus/durability semantics.

Local Go 1.22.4 Linux/amd64, AMD EPYC 9V74, warm filesystem cache, 4 MiB
pseudo-random payload, five iterations per trial, three trials:

| | Old | New |
|---|---:|---:|
| Allocated bytes/op | 42,260,155-42,261,472 | 8,439,032-8,439,070 |
| Allocations/op | 86-89 | 21 |
| Time/op | 21.80-39.34 ms | 5.81-7.10 ms |

Allocation reduction is approximately 80%. The short sequential trials are not
an energy experiment or a production speed guarantee. The fixture excludes the
full blockchain startup and uses the new experimental checkpoint format.

Race tests cover ledger/diagnostics and authenticated malformed/truncated/empty
checkpoint reads. Hardware energy measurement remains opt-in and unmeasured.
No lock-contention diagnosis is supported by the VPS samples. No io_uring,
O_NONBLOCK, writable mmap, signature journal change or runtime migration is
introduced. Read-only mmap page faults may block; a goroutine is not proof of
asynchronous disk I/O. Durable blocks/signatures still require their existing
persistence path independent of optional checkpoint writing.

## Release gates remain open

Full runtime lazy-history integration, independent review, realistic bounded
memory stress/crash testing, cold/warm profile comparison on VPS, all-five-node
agreement and sequential recovery, APoW reward/receipt/finality proof and
separate legacy-freeze/mainnet economic-state migration remain required.
This diagnostic improvement does not authorize or certify mainnet activation.
