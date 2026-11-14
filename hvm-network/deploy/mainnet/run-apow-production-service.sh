#!/usr/bin/env bash
set -euo pipefail

# HVM-R8A: stage and exercise the production APoW worker service boundary.
# This gate never starts/restarts/enables a validator and never activates mainnet.
KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock
APOW_HOST=77.90.188.153
VALIDATOR_SERVICE=hashburst-hvm-mainnet.service
MINER_USER=hashburst-apow-miner
MINER_SERVICE=hashburst-apow-mainnet.service
BASE=/opt/hashburst-apow-mainnet
KEYDIR=/var/lib/hashburst-apow-mainnet
KEY=$KEYDIR/miner.key
BIN=$BASE/hvm-apow-miner
UNIT=/etc/systemd/system/$MINER_SERVICE
ENDPOINT=http://127.0.0.1:18019/apow
CHAIN_ID=4735489
LOG=mainnet-r8a-apow-production-service.log
LOCAL_BIN=./hvm-apow-miner-r8a
SSH=(ssh -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
SCP=(scp -q -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)

fail(){ echo "HVM_R8A_FAIL: $*" | tee -a "$LOG" >&2; exit 1; }
remote(){ SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$APOW_HOST" "$@"; }
copy(){ SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" "$1" "root@$APOW_HOST:$2"; }

[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$KNOWN" && -S "$SOCK" ]] || fail "controlled SSH unavailable"
python3 hvm-network/deploy/mainnet/check-freeze.py | tee "$LOG"
grep -F '"chain_id":4735489' hvm-network/deploy/mainnet/protocol.freeze.json >/dev/null || fail "mainnet chain id changed"
grep -F '"apow":{"gas_limit":15000000,"activation_height":9,"initial_bits":20,"min_bits":16,"max_bits":32,"window":60,"target_seconds":60}' hvm-network/deploy/mainnet/protocol.freeze.json >/dev/null || fail "frozen APoW parameters changed"

go -C hvm-network build -trimpath -o "../$LOCAL_BIN" ./cmd/hvm-apow-miner
MINER_SHA=$(sha256sum "$LOCAL_BIN" | awk '{print $1}')
echo "R8A_MINER_BUILD_OK sha256=$MINER_SHA" | tee -a "$LOG"

# Mutation barrier: validator must already be stopped/disabled before any APoW staging.
remote "set -euo pipefail
 test \"\$(systemctl is-active '$VALIDATOR_SERVICE' || true)\" = inactive
 test \"\$(systemctl is-enabled '$VALIDATOR_SERVICE' || true)\" = disabled
 test ! -L '$BASE'
 test ! -L '$KEYDIR'
 if test -e '$UNIT'; then
   grep -F 'User=$MINER_USER' '$UNIT' >/dev/null
   grep -F -- '--chain-id $CHAIN_ID' '$UNIT' >/dev/null
   grep -F -- '--key-file $KEY' '$UNIT' >/dev/null
 fi"
echo HVM_R8A_VALIDATOR_AT_REST_MUTATION_BARRIER_OK | tee -a "$LOG"

# Prepare only the controlled staging directories before SCP. The validator
# remains stopped/disabled; no key, unit or executable is installed here.
remote "set -euo pipefail
 test \"\$(systemctl is-active '$VALIDATOR_SERVICE' || true)\" = inactive
 test \"\$(systemctl is-enabled '$VALIDATOR_SERVICE' || true)\" = disabled
 getent passwd '$MINER_USER' >/dev/null || useradd --system --home-dir '$KEYDIR' --shell /usr/sbin/nologin '$MINER_USER'
 uid=\$(id -u '$MINER_USER'); gid=\$(id -g '$MINER_USER')
 install -d -m 0755 -o root -g root '$BASE'
 install -d -m 0700 -o '$MINER_USER' -g '$MINER_USER' '$KEYDIR'
 test ! -L '$BASE'; test ! -L '$KEYDIR'
 test \"\$(stat -c '%u:%g:%a' '$BASE')\" = \"0:0:755\"
 test \"\$(stat -c '%u:%g:%a' '$KEYDIR')\" = \"\$uid:\$gid:700\"
 test \"\$(systemctl is-active '$VALIDATOR_SERVICE' || true)\" = inactive
 test \"\$(systemctl is-enabled '$VALIDATOR_SERVICE' || true)\" = disabled"
echo HVM_R8A_STAGING_DIRECTORIES_READY_OK | tee -a "$LOG"

copy "$LOCAL_BIN" "$BASE/.hvm-apow-miner.r8a-candidate"
SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$APOW_HOST" bash -s --  "$MINER_USER" "$MINER_SERVICE" "$BASE" "$KEYDIR" "$KEY" "$BIN" "$UNIT" "$ENDPOINT" "$CHAIN_ID" "$MINER_SHA" "$VALIDATOR_SERVICE" <<'REMOTE'
set -euo pipefail
user=$1; service=$2; base=$3; keydir=$4; key=$5; bin=$6; unit=$7; endpoint=$8; chain=$9; sha=${10}; validator=${11}
test "$(systemctl is-active "$validator" || true)" = inactive
test "$(systemctl is-enabled "$validator" || true)" = disabled
getent passwd "$user" >/dev/null || useradd --system --home-dir "$keydir" --shell /usr/sbin/nologin "$user"
uid=$(id -u "$user"); gid=$(id -g "$user")
install -d -m 0755 -o root -g root "$base"
install -d -m 0700 -o "$user" -g "$user" "$keydir"
test ! -L "$base"; test ! -L "$keydir"
test "$(sha256sum "$base/.hvm-apow-miner.r8a-candidate" | awk '{print $1}')" = "$sha"
chown root:root "$base/.hvm-apow-miner.r8a-candidate"; chmod 0755 "$base/.hvm-apow-miner.r8a-candidate"
mv -f "$base/.hvm-apow-miner.r8a-candidate" "$bin"
test "$(sha256sum "$bin" | awk '{print $1}')" = "$sha"
if test -e "$key"; then
  test ! -L "$key"
  test -f "$key"
  test "$(stat -c '%u:%g' "$key")" = "$uid:$gid"
  test "$(stat -c '%a' "$key")" = 600
else
  runuser -u "$user" -- "$bin" --key-file "$key" --generate-key
fi
test "$(stat -c '%u:%g' "$key")" = "$uid:$gid"
test "$(stat -c '%a' "$key")" = 600
identity=$(runuser -u "$user" -- "$bin" --key-file "$key" --identity-only)
case "$identity" in APOW_MINER_ADDRESS=0x*) ;; *) exit 31 ;; esac
cat > "$unit.tmp" <<EOF
[Unit]
Description=HashBurst HVM mainnet APoW production worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$user
Group=$user
ExecStart=$bin --endpoint $endpoint --key-file $key --chain-id $chain --loop --timeout 120s
Restart=on-failure
RestartSec=5
Nice=15
# E2-Q: cap the reference APoW worker at one CPU equivalent. This changes only
# the service resource budget; APoW consensus parameters and bits remain frozen.
CPUQuota=100%
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6

[Install]
WantedBy=multi-user.target
EOF
if test -e "$unit"; then
  cmp -s "$unit.tmp" "$unit" || { rm -f "$unit.tmp"; echo "existing APoW unit differs; retained" >&2; exit 32; }
  rm -f "$unit.tmp"
else
  chown root:root "$unit.tmp"; chmod 0644 "$unit.tmp"; mv "$unit.tmp" "$unit"
fi
systemd-analyze verify "$unit"
systemctl daemon-reload
systemctl disable "$service" >/dev/null 2>&1 || true
test "$(systemctl is-enabled "$service" || true)" = disabled
test "$(systemctl is-active "$validator" || true)" = inactive
echo "$identity"
echo R8A_APOW_STAGED_DISABLED_OK
REMOTE

IDENTITY=$(remote "runuser -u '$MINER_USER' -- '$BIN' --key-file '$KEY' --identity-only")
echo "HVM_R8A_MINER_IDENTITY_SEPARATE_OK $IDENTITY" | tee -a "$LOG"

# Service lifecycle acceptance with validators still stopped. With --loop, an
# unavailable loopback /apow endpoint is a transient wait condition, so the
# worker can be exercised without producing/submitting work or activating chain.
remote "set -euo pipefail
 test \"\$(systemctl is-active '$VALIDATOR_SERVICE' || true)\" = inactive
 systemctl start '$MINER_SERVICE'
 sleep 3
 systemctl is-active --quiet '$MINER_SERVICE'
 test \"\$(systemctl is-enabled '$MINER_SERVICE' || true)\" = disabled
 pid=\$(systemctl show -p MainPID --value '$MINER_SERVICE')
 test \"\$pid\" -gt 1
 kill -KILL \"\$pid\"
 for i in \$(seq 1 20); do
   sleep 1
   new=\$(systemctl show -p MainPID --value '$MINER_SERVICE')
   if systemctl is-active --quiet '$MINER_SERVICE' && test \"\$new\" -gt 1 && test \"\$new\" != \"\$pid\"; then
     echo R8A_APOW_RESTART_RECOVERY_OK old_pid=\$pid new_pid=\$new
     break
   fi
 done
 new=\$(systemctl show -p MainPID --value '$MINER_SERVICE')
 test \"\$new\" -gt 1
 test \"\$new\" != \"\$pid\"
 systemctl stop '$MINER_SERVICE'
 systemctl disable '$MINER_SERVICE' >/dev/null 2>&1 || true
 test \"\$(systemctl is-active '$MINER_SERVICE' || true)\" = inactive
 test \"\$(systemctl is-enabled '$MINER_SERVICE' || true)\" = disabled
 test \"\$(systemctl is-active '$VALIDATOR_SERVICE' || true)\" = inactive
 test \"\$(systemctl is-enabled '$VALIDATOR_SERVICE' || true)\" = disabled"
echo HVM_R8A_APOW_SERVICE_RESTART_RECOVERY_OK | tee -a "$LOG"

UNIT_SHA=$(remote "sha256sum '$UNIT' | awk '{print \$1}'")
KEY_MODE=$(remote "stat -c '%a' '$KEY'")
cat > mainnet-r8a-apow-production-service.json <<JSON
{
  "schema": "hashburst-hvm-mainnet-r8a-apow-production-service-v1",
  "chain_id": 4735489,
  "source_sha": "$GITHUB_SHA",
  "host": "$APOW_HOST",
  "miner_binary_sha256": "$MINER_SHA",
  "miner_identity": "${IDENTITY#APOW_MINER_ADDRESS=}",
  "unit_sha256": "$UNIT_SHA",
  "key_mode": "$KEY_MODE",
  "validator_service_started": false,
  "miner_service_enabled": false,
  "production_activation_authorized": false,
  "founder_private_key_used": false,
  "validator_private_key_used": false
}
JSON
chmod 0444 mainnet-r8a-apow-production-service.json
sha256sum mainnet-r8a-apow-production-service.json | tee -a "$LOG"
{
 echo HVM_R8A_APOW_PRODUCTION_SERVICE_STAGED_OK
 echo HVM_R8A_APOW_CUSTODY_BOUNDARY_OK
 echo HVM_R8A_APOW_RESTART_RECOVERY_ACCEPTANCE_OK
 echo HVM_R8A_VALIDATOR_REMAINS_STOPPED_DISABLED_OK
 echo NO_APOW_PROOF_SUBMITTED
 echo NO_PRODUCTION_ACTIVATION
 echo FOUNDER_PRIVATE_KEY_NOT_USED
 echo VALIDATOR_PRIVATE_KEY_NOT_USED
} | tee -a "$LOG"
