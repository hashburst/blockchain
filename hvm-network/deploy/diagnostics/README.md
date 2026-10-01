# Read-only sampling and Go profiles

`sample-node.py` can run now on an existing Linux VPS. It does not restart services,
read wallet secrets, alter configuration or send transactions:

```sh
python3 sample-node.py --unit hashburst-hvm-testnet.service --seconds 60 > node-sample.jsonl
```

For the observer use `hashburst-hvm-testnet-ingress.service`. Collect during the
slow phase when possible. If the process has finished recovery, this describes
steady operation only; it cannot retrospectively identify replay CPU hotspots.
Cgroup v1/missing counters are reported unavailable rather than synthesized.

The NEW development binary accepts these optional flags, before state recovery:

```
--profile-dir /a/new/private/directory --profile-kind cpu --profile-duration 60s
```

The parent directory must exist and be writable by the service user; the output
must not exist. Profiling is off by default. Do not add these flags to an old
binary; do not launch another node on the live state. The flags belong in the
reviewed service command during a planned rollout, not a parallel ad-hoc process.
The initial window profiles the start of recovery, not necessarily its later replay
phase. Targeted phase/window capture is needed before attributing later hotspots.
Other kinds: heap, block, mutex, trace. Use trace for about 5s; maximum window 5m.
No HTTP profiling port is exposed. Profiling files may disclose code paths and
workload metadata; keep them private. Automatic completion closes the profile,
while shutdown also drains it.

Analyze with the matching Go toolchain and exact candidate binary:

```sh
go tool pprof -top ./hashburst-testnet ./profile/cpu.pprof
go tool pprof -top -alloc_space ./hashburst-testnet ./profile/heap.pprof
go tool pprof -top -inuse_space ./hashburst-testnet ./profile/heap.pprof
go tool pprof -top ./hashburst-testnet ./profile/block.pprof
go tool pprof -top ./hashburst-testnet ./profile/mutex.pprof
go tool trace ./profile/trace.pprof
```

Use independent captures for the different profile kinds. No result from these
commands is included until a real VPS capture is supplied. Do not translate a
local microbenchmark directly into lower VPS energy use or faster finality.
