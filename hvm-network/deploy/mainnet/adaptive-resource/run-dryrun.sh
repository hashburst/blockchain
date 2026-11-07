#!/usr/bin/env bash
set -euo pipefail
KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
OUT=mainnet-orchestrator-dryrun.json
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
printf '{"schema":1,"mode":"dry-run","decisions":[' > "$OUT"; first=1
for ip in 77.90.188.153 77.90.188.154; do
 ssh -T -i "$KEY" -o IdentitiesOnly=yes -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes -o ConnectTimeout=10 root@"$ip" 'python3 - <<'"'"'PY'"'"'
import json,os,subprocess
def out(a):
 return subprocess.run(a,text=True,capture_output=True,timeout=5).stdout.strip()
def active(u): return out(["systemctl","is-active",u])
def pct(comm):
 s=0.0
 for line in out(["ps","-C",comm,"-o","pcpu="]).splitlines():
  try:s+=float(line)
  except:pass
 return s
load=float(open("/proc/loadavg").read().split()[0])
print(json.dumps({"node":out(["hostname"]),"cpu_count":os.cpu_count(),"load1":load,
 "miner_cpu_percent":pct("cpuminer"),"hvm_cpu_percent":pct("hashburst-testn"),"tep_cpu_percent":pct("python3"),
 "services":{"hashburst-hvm-testnet.service":active("hashburst-hvm-testnet.service"),"hashburst-tep.service":active("hashburst-tep.service")},
 "energy_measurement_quality":"proxy"},separators=(",",":")))
PY' > "$TMP/telemetry.json"
 python3 hvm-network/deploy/mainnet/adaptive-resource/orchestrator.py < "$TMP/telemetry.json" > "$TMP/decision.json"
 if [ "$first" -eq 0 ]; then printf ',' >> "$OUT"; fi
 cat "$TMP/decision.json" >> "$OUT"; first=0
done
printf ']}
' >> "$OUT"
python3 - "$OUT" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert len(d["decisions"])==2
for x in d["decisions"]:
 assert x["mode"]=="dry-run" and x["proposal"]["actuation"]=="none"
 assert x["protected"]["services"]["hashburst-hvm-testnet.service"]=="active"
 assert x["protected"]["services"]["hashburst-tep.service"]=="active"
print(json.dumps(d,indent=2))
PY
echo HASHBURST_ADAPTIVE_RESOURCE_TELEMETRY_2OF2_OK
echo HASHBURST_ADAPTIVE_RESOURCE_AI_DECISION_DRYRUN_2OF2_OK
echo HASHBURST_ADAPTIVE_RESOURCE_SERVICE_PROTECTION_2OF2_OK
echo HASHBURST_ADAPTIVE_RESOURCE_ORCHESTRATION_DRYRUN_2OF2_OK
