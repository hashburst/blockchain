#!/usr/bin/env bash
set -euo pipefail
EXPECTED_NAME=FLEET-FREEZE-HA-1791220850167915000.json
EXPECTED_SHA=d0f2246fc26e9e72c6117ac0d6f6805c7427a8cc1b106613c822a1cbb7e0a6f6
DEST_DIR=/var/lib/hashburst-actions/mainnet-evidence
DEST="$DEST_DIR/$EXPECTED_NAME"
SSH_KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157 64.31.4.9)
SSH=(ssh -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
SCP=(scp -q -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
TMP="$(mktemp -d /var/lib/hashburst-actions/freeze-recovery.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT
log(){ echo "$*" | tee -a mainnet-freeze-evidence-recovery.log; }
fail(){ log "FREEZE_EVIDENCE_RECOVERY_FAIL: $*"; exit 1; }
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$SSH_KEY" && -r "$KNOWN" ]] || fail "fleet SSH material unavailable"
if [[ -f "$DEST" ]]; then
  got="$(sha256sum "$DEST" | awk '{print $1}')"
  [[ "$got" == "$EXPECTED_SHA" ]] || fail "canonical destination exists with wrong digest sha256=$got"
  log "FREEZE_EVIDENCE_ALREADY_CANONICAL sha256=$got"
  exit 0
fi
while IFS= read -r -d '' p; do
  got="$(sha256sum "$p" 2>/dev/null | awk '{print $1}' || true)"
  log "FREEZE_EVIDENCE_LOCAL_CANDIDATE path=$p sha256=$got"
  if [[ "$got" == "$EXPECTED_SHA" ]]; then
    install -d -m 0700 "$DEST_DIR"
    install -m 0600 "$p" "$DEST"
    [[ "$(sha256sum "$DEST" | awk '{print $1}')" == "$EXPECTED_SHA" ]] || fail "post-copy local digest mismatch"
    log "FREEZE_EVIDENCE_RECOVERED source=coordinator sha256=$EXPECTED_SHA"
    exit 0
  fi
done < <(find /var/lib/hashburst-actions /opt /tmp -xdev -type f -name "$EXPECTED_NAME" -print0 2>/dev/null)
for ip in "${HOSTS[@]}"; do
  log "FREEZE_EVIDENCE_REMOTE_SCAN_BEGIN host=$ip"
  mapfile -t rows < <("${SSH[@]}" "root@$ip" "find /var/lib /opt /root /tmp -xdev -type f -name '$EXPECTED_NAME' -exec sha256sum {} \\; 2>/dev/null" || true)
  for row in "${rows[@]}"; do
    got="${row%% *}"
    path="${row#*  }"
    log "FREEZE_EVIDENCE_REMOTE_CANDIDATE host=$ip path=$path sha256=$got"
    [[ "$got" == "$EXPECTED_SHA" ]] || continue
    candidate="$TMP/$ip.$EXPECTED_NAME"
    "${SCP[@]}" "root@$ip:$path" "$candidate" || fail "copy from matching remote candidate failed host=$ip"
    [[ "$(sha256sum "$candidate" | awk '{print $1}')" == "$EXPECTED_SHA" ]] || fail "downloaded candidate digest mismatch host=$ip"
    install -d -m 0700 "$DEST_DIR"
    install -m 0600 "$candidate" "$DEST"
    [[ "$(sha256sum "$DEST" | awk '{print $1}')" == "$EXPECTED_SHA" ]] || fail "canonical post-copy digest mismatch"
    log "FREEZE_EVIDENCE_RECOVERED source=$ip sha256=$EXPECTED_SHA"
    exit 0
  done
done
fail "no exact SHA-256 match found on coordinator or managed fleet"
