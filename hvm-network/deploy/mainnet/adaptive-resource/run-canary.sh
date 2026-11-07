#!/usr/bin/env bash
set -euo pipefail
KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
CANARY=77.90.188.153
CONTROL=77.90.188.154
WINDOW="${HASHBURST_CANARY_WINDOW_SECONDS:-120}"
TARGET_QUOTA=1200
SSH=(ssh -T -i "$KEY" -o IdentitiesOnly=yes -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes -o ConnectTimeout=10)
OUT=mainnet-actuator-canary.json
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
sample(){
 local ip="$1" dst="$2"
 "${SSH[@]}" root@"$ip" 'python3 -' < hvm-network/deploy/mainnet/adaptive-resource/sample-node.py > "$dst"
}
sample "$CANARY" "$TMP/canary.before"; sample "$CONTROL" "$TMP/control.before"
python3 - "$TMP/canary.before" "$TMP/control.before" <<'PY'
import json,sys
for p in sys.argv[1:]:
 d=json.load(open(p))
 assert d["hvm"]["state"]=="active" and d["tep"]["state"]=="active"
PY
# Canary-only mutation. Scope is a transient systemd scope attached to the existing cpuminer PID.
# No miner restart and no persistent unit file is written.
"${SSH[@]}" root@"$CANARY" "bash -se" <<'REMOTE'
set -euo pipefail
pid="$(pgrep -xo cpuminer)"
test -n "$pid"
systemctl is-active --quiet hashburst-hvm-testnet.service
systemctl is-active --quiet hashburst-tep.service
mkdir -p /run/hashburst-canary
printf '%s\n' "$pid" > /run/hashburst-canary/cpuminer.pid
printf '%s\n' "$(ps -o ni= -p "$pid" | tr -d ' ')" > /run/hashburst-canary/previous.nice
renice -n 10 -p "$pid" >/dev/null
systemd-run --unit=hashburst-mining-canary --scope --slice=system.slice -p CPUQuota=1200% --pid="$pid" /bin/true >/dev/null
systemctl show hashburst-mining-canary.scope -p CPUQuotaPerSecUSec -p ActiveState > /run/hashburst-canary/applied.state
REMOTE
sleep "$WINDOW"
sample "$CANARY" "$TMP/canary.after"; sample "$CONTROL" "$TMP/control.after"
rollback=0
if ! "${SSH[@]}" root@"$CANARY" 'systemctl is-active --quiet hashburst-hvm-testnet.service && systemctl is-active --quiet hashburst-tep.service'; then rollback=1; fi
if [ "$rollback" -eq 1 ]; then
 "${SSH[@]}" root@"$CANARY" 'bash -se' <<'REMOTE'
set -euo pipefail
pid="$(cat /run/hashburst-canary/cpuminer.pid)"
oldnice="$(cat /run/hashburst-canary/previous.nice)"
systemctl stop hashburst-mining-canary.scope || true
renice -n "$oldnice" -p "$pid" >/dev/null || true
REMOTE
fi
python3 - "$TMP/canary.before" "$TMP/canary.after" "$TMP/control.before" "$TMP/control.after" "$rollback" > "$OUT" <<'PY'
import json,sys,hashlib
cb,ca,xb,xa=[json.load(open(p)) for p in sys.argv[1:5]]; rb=bool(int(sys.argv[5]))
def delta(a,b,k): return b[k]-a[k]
r={"schema":"hashburst-canary-comparison-v1","canary_ip":"77.90.188.153","control_ip":"77.90.188.154",
"target":{"cpu_quota_percent":1200,"nice":10,"miner_restart":False},
"canary":{"before":cb,"after":ca,"load_delta":delta(cb,ca,"load1"),"miner_cpu_delta":delta(cb,ca,"miner_cpu_percent")},
"control":{"before":xb,"after":xa,"load_delta":delta(xb,xa,"load1"),"miner_cpu_delta":delta(xb,xa,"miner_cpu_percent")},
"rollback":rb,"status":"rolled_back" if rb else "canary_active"}
body=json.dumps(r,sort_keys=True,separators=(",",":"));r["comparison_sha256"]=hashlib.sha256(body.encode()).hexdigest()
print(json.dumps(r,sort_keys=True,separators=(",",":")))
PY
python3 - "$OUT" <<'PY'
import json
d=json.load(open("mainnet-actuator-canary.json"))
assert d["control"]["before"]["hvm"]["state"]=="active" and d["control"]["after"]["hvm"]["state"]=="active"
assert d["control"]["before"]["tep"]["state"]=="active" and d["control"]["after"]["tep"]["state"]=="active"
assert d["canary"]["after"]["hvm"]["state"]=="active" and d["canary"]["after"]["tep"]["state"]=="active"
print(json.dumps(d,indent=2))
PY
echo HASHBURST_ADAPTIVE_RESOURCE_CANARY_TELEMETRY_OK
echo HASHBURST_ADAPTIVE_RESOURCE_CONTROL_UNCHANGED_OK
echo HASHBURST_ADAPTIVE_RESOURCE_CANARY_SERVICE_PROTECTION_OK
echo HASHBURST_ADAPTIVE_RESOURCE_CANARY_APPLY_1OF2_OK
