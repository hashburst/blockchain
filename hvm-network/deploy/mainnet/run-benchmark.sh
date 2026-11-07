#!/usr/bin/env bash
set -euo pipefail

KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
NODES=(
  "77.90.188.153 validator"
  "77.90.188.154 validator"
  "77.90.188.155 validator"
  "77.90.188.157 validator"
  "64.31.4.9 observer"
)

test -r "$KEY"
test -r "$KNOWN"

: > mainnet-benchmark.log
printf '{"schema":1,"chain_id":4735489,"mode":"read-only","nodes":[' > mainnet-benchmark.json
first=1
ok=1

for entry in "${NODES[@]}"; do
  read -r ip role <<<"$entry"
  echo "BENCHMARK_READONLY_BEGIN ip=$ip role=$role" | tee -a mainnet-benchmark.log
  start=$(date +%s%N)
  if out=$(ssh -T -i "$KEY" -o "UserKnownHostsFile=$KNOWN" -o IdentitiesOnly=yes -o ControlMaster=no -o ControlPath=none -o ControlPersist=no -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 "root@$ip" 'python3 - <<'"'"'PY'"'"'
import json,os,platform,time
from pathlib import Path
t=time.monotonic_ns()
payload={
 "ok": True,
 "hostname": platform.node(),
 "architecture": platform.machine(),
 "cpu_count": os.cpu_count(),
 "loadavg": list(os.getloadavg()),
 "mem_available_kib": None,
 "root_free_bytes": os.statvfs("/").f_bavail * os.statvfs("/").f_frsize,
 "mainnet_paths_present": [p for p in ("/etc/hashburst-hvm-mainnet","/var/lib/hashburst-hvm-mainnet","/root/hvm-mainnet-identity") if Path(p).exists() or Path(p).is_symlink()],
}
try:
 for line in Path("/proc/meminfo").read_text().splitlines():
  if line.startswith("MemAvailable:"):
   payload["mem_available_kib"]=int(line.split()[1]);break
except Exception:
 pass
payload["probe_elapsed_ns"]=time.monotonic_ns()-t
print(json.dumps(payload,separators=(",",":")))
PY
'); then
    end=$(date +%s%N)
    elapsed=$((end-start))
    echo "BENCHMARK_READONLY_OK ip=$ip ssh_elapsed_ns=$elapsed" | tee -a mainnet-benchmark.log
    node=$(python3 -c 'import json,sys; x=json.loads(sys.argv[1]); x["expected_ip"]=sys.argv[2]; x["role"]=sys.argv[3]; x["ssh_elapsed_ns"]=int(sys.argv[4]); print(json.dumps(x,separators=(",",":")))' "$out" "$ip" "$role" "$elapsed")
  else
    ok=0
    echo "BENCHMARK_READONLY_FAIL ip=$ip" | tee -a mainnet-benchmark.log
    node=$(python3 -c 'import json,sys; print(json.dumps({"ok":False,"expected_ip":sys.argv[1],"role":sys.argv[2],"error":"ssh probe failed"},separators=(",",":")))' "$ip" "$role")
  fi
  if [ "$first" -eq 0 ]; then printf ',' >> mainnet-benchmark.json; fi
  first=0
  printf '%s' "$node" >> mainnet-benchmark.json
done

printf '],"ok":%s}\n' "$([ "$ok" -eq 1 ] && echo true || echo false)" >> mainnet-benchmark.json
if [ "$ok" -eq 1 ]; then
  echo "HVM_MAINNET_BENCHMARK_READONLY_5OF5_OK" | tee -a mainnet-benchmark.log
  exit 0
fi
echo "HVM_MAINNET_BENCHMARK_READONLY_FAILED" | tee -a mainnet-benchmark.log
exit 1
