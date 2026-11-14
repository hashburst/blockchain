#!/usr/bin/env bash
set -euo pipefail

# HashBurst HVM mainnet E3 reward-accounting gate.
# Operates only on an already acquired local ledger snapshot.
# It never connects to validators, submits transactions/APoW, restarts services,
# or reads private-key contents.

usage() {
  cat <<'EOF'
Usage:
  sudo env \
    E3_REPO=/path/to/blockchain \
    E3_EXPECTED_MASTER=<sha> \
    E3_OUT=/path/to/evidence/economic-liveness-e3-reward \
    E3_AUDITOR=/path/to/hvm-apow-audit \
    E3_DAT_SHA256=<sha256> \
    E3_IDX_SHA256=<sha256> \
    E3_AUDITOR_SHA256=<sha256> \
    ./run-e3-reward-accounting-readonly.sh

Optional:
  E3_RUNNER_USER        default: hashburst-actions
  E3_CHAIN_ID           default: 4735489
  E3_HEIGHT1            default: 1638
  E3_HEIGHT2            default: 1639
  E3_EXPECTED_HASH1
  E3_EXPECTED_PARENT1
  E3_EXPECTED_BENEFICIARY1

The script requires the 'acl' package (setfacl/getfacl). If ACL changes are
needed, it displays the exact target and asks for explicit authorization.
EOF
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "ERROR: required environment variable missing: $name" >&2
    usage >&2
    exit 2
  fi
}

[[ "${EUID}" -eq 0 ]] || { echo "ERROR: run as root" >&2; exit 2; }

for v in E3_REPO E3_EXPECTED_MASTER E3_OUT E3_AUDITOR E3_DAT_SHA256 E3_IDX_SHA256 E3_AUDITOR_SHA256; do
  require_env "$v"
done

RUNNER="${E3_RUNNER_USER:-hashburst-actions}"
CHAIN_ID="${E3_CHAIN_ID:-4735489}"
HEIGHT1="${E3_HEIGHT1:-1638}"
HEIGHT2="${E3_HEIGHT2:-1639}"
SNAP="$E3_OUT/mainnet-ledger-snapshot"
DAT="$SNAP/blockchain.dat"
IDX="$SNAP/blockchain.idx"
JSON1="$E3_OUT/audit-$HEIGHT1.json"
JSON2="$E3_OUT/audit-$HEIGHT2.json"

for cmd in git sha256sum python3 namei runuser setfacl getfacl; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "ERROR: required command unavailable: $cmd" >&2
    if [[ "$cmd" == "setfacl" || "$cmd" == "getfacl" ]]; then
      echo "Install prerequisite first: apt-get install --no-install-recommends acl" >&2
    fi
    exit 21
  }
done

id "$RUNNER" >/dev/null 2>&1 || { echo "ERROR: runner user not found: $RUNNER" >&2; exit 2; }

echo "=== E3 READ-ONLY REWARD ACCOUNTING ==="
echo "UTC=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

echo "=== A. SOURCE BINDING ==="
HEAD="$(runuser -u "$RUNNER" -- git -C "$E3_REPO" rev-parse HEAD)"
ORIGIN_MASTER="$(runuser -u "$RUNNER" -- git -C "$E3_REPO" rev-parse origin/master)"
echo "HEAD=$HEAD"
echo "ORIGIN_MASTER=$ORIGIN_MASTER"
echo "EXPECTED_MASTER=$E3_EXPECTED_MASTER"
[[ "$HEAD" == "$E3_EXPECTED_MASTER" ]]
[[ "$ORIGIN_MASTER" == "$E3_EXPECTED_MASTER" ]]
echo "E3_EXACT_MASTER_OK"

echo "=== B. EVIDENCE HASH BINDING ==="
[[ -f "$DAT" && -f "$IDX" && -f "$E3_AUDITOR" ]]
DAT_SHA="$(sha256sum "$DAT" | awk '{print $1}')"
IDX_SHA="$(sha256sum "$IDX" | awk '{print $1}')"
AUDITOR_SHA="$(sha256sum "$E3_AUDITOR" | awk '{print $1}')"
echo "DAT_SHA256=$DAT_SHA"
echo "IDX_SHA256=$IDX_SHA"
echo "AUDITOR_SHA256=$AUDITOR_SHA"
[[ "$DAT_SHA" == "$E3_DAT_SHA256" ]]
[[ "$IDX_SHA" == "$E3_IDX_SHA256" ]]
[[ "$AUDITOR_SHA" == "$E3_AUDITOR_SHA256" ]]
echo "E3_EVIDENCE_HASH_PINNED_OK"

echo "=== C. PERMISSION DIAGNOSTIC ==="
namei -l "$DAT"
DAT_READABLE=no
IDX_READABLE=no
OUT_WRITABLE=no
runuser -u "$RUNNER" -- test -r "$DAT" && DAT_READABLE=yes || true
runuser -u "$RUNNER" -- test -r "$IDX" && IDX_READABLE=yes || true
runuser -u "$RUNNER" -- test -w "$E3_OUT" && OUT_WRITABLE=yes || true
echo "DAT_READABLE=$DAT_READABLE"
echo "IDX_READABLE=$IDX_READABLE"
echo "OUT_WRITABLE=$OUT_WRITABLE"

if [[ "$DAT_READABLE" != yes || "$IDX_READABLE" != yes || "$OUT_WRITABLE" != yes ]]; then
  echo "AUTHORIZATION_REQUIRED=yes"
  echo "Proposed local ACL target: $E3_OUT"
  echo "Grant to $RUNNER: rwx on E3_OUT only."
  echo "No validator, service, key, ledger content, or testnet mutation."
  read -r -p "Authorize this local ACL change? Type exactly YES: " AUTH
  if [[ "$AUTH" != "YES" ]]; then
    echo "AUTHORIZATION_GRANTED=no"
    echo "NO_PERMISSION_MUTATION_EXECUTED"
    exit 22
  fi
  echo "AUTHORIZATION_GRANTED=yes"
  setfacl -m "u:$RUNNER:rwx" "$E3_OUT"
  echo "AUTHORIZED_LOCAL_ACL_CHANGE_EXECUTED=yes"
fi

runuser -u "$RUNNER" -- test -r "$DAT"
runuser -u "$RUNNER" -- test -r "$IDX"
runuser -u "$RUNNER" -- test -w "$E3_OUT"
runuser -u "$RUNNER" -- test -x "$E3_AUDITOR"
echo "HASHBURST_ACTIONS_REQUIRED_ACCESS_OK"

# ACL changes must not alter evidence bytes.
[[ "$(sha256sum "$DAT" | awk '{print $1}')" == "$E3_DAT_SHA256" ]]
[[ "$(sha256sum "$IDX" | awk '{print $1}')" == "$E3_IDX_SHA256" ]]
[[ "$(sha256sum "$E3_AUDITOR" | awk '{print $1}')" == "$E3_AUDITOR_SHA256" ]]
echo "E3_POST_PERMISSION_CONTENT_INTEGRITY_OK"

audit_height() {
  local height="$1" json="$2"
  runuser -u "$RUNNER" -- "$E3_AUDITOR" \
    --data-dir "$SNAP" --height "$height" --chain-id "$CHAIN_ID" > "$json"
  python3 -m json.tool "$json"
}

echo "=== D. AUDIT HEIGHT $HEIGHT1 ==="
rm -f "$JSON1" "$JSON2"
audit_height "$HEIGHT1" "$JSON1"

python3 - "$JSON1" "$CHAIN_ID" "$HEIGHT1" \
  "${E3_EXPECTED_HASH1:-}" "${E3_EXPECTED_PARENT1:-}" "${E3_EXPECTED_BENEFICIARY1:-}" <<'PY'
import json, sys
path, chain, height, expected_hash, expected_parent, expected_beneficiary = sys.argv[1:]
with open(path, encoding="utf-8") as f:
    x = json.load(f)
assert int(x["chain_id"]) == int(chain), x
assert int(x["height"]) == int(height), x
if expected_hash:
    assert x["hash"].lower() == expected_hash.lower(), x
if expected_parent:
    assert x["parent_hash"].lower() == expected_parent.lower(), x
if expected_beneficiary:
    assert x["beneficiary"].lower() == expected_beneficiary.lower(), x
assert int(x["system_reward_count"]) == 1, x
assert int(x["reward_hbt"]) == 50, x
assert int(x["reward_units"]) == 5000000000, x
assert x["signature_and_work_verified"] is True, x
assert x.get("reward_tx"), x
assert x.get("certificate") is not None, x
assert x.get("proof") is not None, x
print("HEIGHT1_REWARD_AND_FINALITY_OK")
PY

echo "=== E. AUDIT HEIGHT $HEIGHT2 ==="
audit_height "$HEIGHT2" "$JSON2"

python3 - "$JSON2" "$CHAIN_ID" "$HEIGHT2" <<'PY'
import json, sys
path, chain, height = sys.argv[1:]
with open(path, encoding="utf-8") as f:
    x = json.load(f)
assert int(x["chain_id"]) == int(chain), x
assert int(x["height"]) == int(height), x
assert int(x["system_reward_count"]) == 1, x
assert int(x["reward_hbt"]) == 50, x
assert int(x["reward_units"]) == 5000000000, x
assert x["signature_and_work_verified"] is True, x
assert x.get("beneficiary"), x
assert x.get("reward_tx"), x
assert x.get("certificate") is not None, x
assert x.get("proof") is not None, x
print("HEIGHT2_REWARD_AND_FINALITY_OK")
PY

echo "=== F. SAFETY / ACCEPTANCE ==="
echo "VALIDATOR_CONNECTIONS=0"
echo "PRIVATE_KEY_CONTENT_READ=no"
echo "TRANSACTION_SUBMISSION=no"
echo "APOW_SUBMISSION=no"
echo "SERVICE_RESTART=no"
echo "CONFIG_CHANGE=no"
echo "LEDGER_MUTATION=no"
echo "REMOTE_FILES_WRITTEN=0"
echo "TESTNET_MUTATION=no"
echo "E3_APOW_REWARD_ACCOUNTING_OK"
echo "E3_READONLY_MAINNET_LEDGER_AUDIT_OK"
sha256sum "$E3_AUDITOR" "$DAT" "$IDX" "$JSON1" "$JSON2"
