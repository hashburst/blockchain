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
# Recover only a previous interrupted canary's nice value, if the same miner PID is still alive.
# This repairs run 42392714865 safely before attempting a new mutation.
"${SSH[@]}" root@"$CANARY" 'bash -se' <<'REMOTE'
set -euo pipefail
if test -r /run/hashburst-canary/cpuminer.pid && test -r /run/hashburst-canary/previous.nice; then
 oldpid="$(cat /run/hashburst-canary/cpuminer.pid)"
 curpid="$(pgrep -xo cpuminer || true)"
 if test -n "$curpid" && test "$oldpid" = "$curpid"; then
  oldnice="$(cat /run/hashburst-canary/previous.nice)"
  renice -n "$oldnice" -p "$curpid" >/dev/null || true
 fi
fi
REMOTE
# Canary-only mutation. Use the existing systemd unit that owns cpuminer and apply a runtime
# cgroup CPUQuota; do not restart the miner and do not write a persistent unit file.
"${SSH[@]}" root@"$CANARY" "bash -se" <<'REMOTE'
set -euo pipefail
pid="$(pgrep -xo cpuminer)"
test -n "$pid"
systemctl is-active --quiet hashburst-hvm-testnet.service
systemctl is-active --quiet hashburst-tep.service
cg="$(awk -F: '$1=="0"{print $3}' "/proc/$pid/cgroup")"
test -n "$cg"
unit="$(basename "$cg")"
case "$unit" in
 *.service|*.scope) ;;
 *) echo "cpuminer is not owned by a dedicated systemd service/scope: $cg" >&2; exit 65 ;;
esac
systemctl show "$unit" -p LoadState --value | grep -Fx active >/dev/null || {
 # LoadState is normally "loaded"; retain a precise diagnostic instead of mutating.
 load="$(systemctl show "$unit" -p LoadState --value || true)"
 test "$load" = "loaded" || { echo "cpuminer systemd unit is not loaded: $unit ($load)" >&2; exit 66; }
}
quota_before="$(systemctl show "$unit" -p CPUQuotaPerSecUSec --value)"
nice_before="$(ps -o ni= -p "$pid" | tr -d ' ')"
mkdir -p /run/hashburst-canary
printf '%s\n' "$pid" > /run/hashburst-canary/cpuminer.pid
printf '%s\n' "$unit" > /run/hashburst-canary/cpuminer.unit
printf '%s\n' "$nice_before" > /run/hashburst-canary/previous.nice
printf '%s\n' "$quota_before" > /run/hashburst-canary/previous.quota
rollback_apply(){
 systemctl set-property --runtime "$unit" CPUQuota=infinity >/dev/null 2>&1 || true
 renice -n "$nice_before" -p "$pid" >/dev/null 2>&1 || true
}
trap rollback_apply ERR
systemctl set-property --runtime "$unit" CPUQuota=1200%
renice -n 10 -p "$pid" >/dev/null
trap - ERR
systemctl show "$unit" -p CPUQuotaPerSecUSec -p ActiveState -p LoadState > /run/hashburst-canary/applied.state
grep -q '^CPUQuotaPerSecUSec=12s$' /run/hashburst-canary/applied.state || {
 echo "CPUQuota verification failed for $unit" >&2
 rollback_apply
 exit 67
}
REMOTE
sleep "$WINDOW"
sample "$CANARY" "$TMP/canary.after"; sample "$CONTROL" "$TMP/control.after"
rollback=0
if ! "${SSH[@]}" root@"$CANARY" 'systemctl is-active --quiet hashburst-hvm-testnet.service && systemctl is-active --quiet hashburst-tep.service'; then rollback=1; fi
if [ "$rollback" -eq 1 ]; then
 "${SSH[@]}" root@"$CANARY" 'bash -se' <<'REMOTE'
set -euo pipefail
pid="$(cat /run/hashburst-canary/cpuminer.pid)"
unit="$(cat /run/hashburst-canary/cpuminer.unit)"
oldnice="$(cat /run/hashburst-canary/previous.nice)"
systemctl set-property --runtime "$unit" CPUQuota=infinity || true
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
