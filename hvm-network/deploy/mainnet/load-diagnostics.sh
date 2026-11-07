#!/usr/bin/env bash
set -euo pipefail
KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
OUT=mainnet-load-diagnostics.json
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
printf '{"schema":1,"mode":"read-only","nodes":[' > "$OUT"
first=1
for ip in 77.90.188.153 77.90.188.154; do
  ssh -T -i "$KEY" -o IdentitiesOnly=yes -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes -o ConnectTimeout=10 root@"$ip" 'python3 - <<'"'"'PY'"'"'
import json,os,subprocess
def run(args):
 p=subprocess.run(args,text=True,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
 return p.stdout.strip()
rows=[]
for line in run(["ps","-eo","pid,ppid,pcpu,pmem,ni,comm,args","--sort=-pcpu"]).splitlines()[1:31]:
 rows.append(line)
units=[]
for line in run(["systemctl","list-units","--type=service","--state=running","--no-legend","--no-pager"]).splitlines():
 if any(x in line.lower() for x in ("hashburst","hvm","tep","miner","mining")): units.append(line)
print(json.dumps({"hostname":run(["hostname"]),"loadavg":open("/proc/loadavg").read().strip(),"cpu_count":os.cpu_count(),"top_cpu":rows,"hashburst_units":units},separators=(",",":")))
PY' > "$TMP/$ip.json"
  if [ "$first" -eq 0 ]; then printf ',' >> "$OUT"; fi
  cat "$TMP/$ip.json" >> "$OUT"; first=0
done
printf ']}
' >> "$OUT"
cat "$OUT"
echo HVM_HASHBURST_LOAD_DIAGNOSTICS_READONLY_2OF2_OK
