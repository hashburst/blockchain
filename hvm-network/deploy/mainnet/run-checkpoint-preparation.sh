#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
KNOWN=/etc/hashburst-control/fleet_known_hosts
: "${SSH_AUTH_SOCK:=/run/hashburst-control/ssh-agent.sock}"
EVIDENCE_ROOT=/var/lib/hashburst-actions/mainnet-evidence
FREEZE="$EVIDENCE_ROOT/FLEET-FREEZE-HA-1791220850167915000.json"
REATTEST="$DEPLOY/legacy-freeze.reattestation.v1.json"
SOURCE="$EVIDENCE_ROOT/legacy-terminal-generation"
WORK="$ROOT/../mainnet-checkpoint-preparation"
OUT="$WORK/economic-checkpoint"
IDENTITIES="$WORK/identities"
EXPECTED_FREEZE=d0f2246fc26e9e72c6117ac0d6f6805c7427a8cc1b106613c822a1cbb7e0a6f6
fail(){ echo "CHECKPOINT_PREPARATION_FAIL: $*" | tee -a mainnet-checkpoint-preparation.log >&2; exit 1; }
on_err(){ rc=$?; echo "CHECKPOINT_PREPARATION_UNEXPECTED_EXIT rc=$rc line=$1 command=$2" | tee -a mainnet-checkpoint-preparation.log >&2; exit "$rc"; }
trap 'on_err "$LINENO" "$BASH_COMMAND"' ERR
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$KNOWN" && -S "$SSH_AUTH_SOCK" ]] || fail "fleet SSH agent or known_hosts unavailable"
ssh-add -l | grep -F 'SHA256:yvsyCOCv11WL/oeoxb6uo+Bb/yV46ouxhshw8ohqS1Y' >/dev/null || fail "pinned fleet SSH identity unavailable from agent"
python3 "$DEPLOY/check-freeze.py" | tee mainnet-checkpoint-freeze.log
GO_BIN="$(command -v go || true)"
[[ -n "$GO_BIN" ]] || fail "Go unavailable"
GO_VERSION="$("$GO_BIN" env GOVERSION 2>/dev/null || true)"
echo "CHECKPOINT_COORDINATOR_GO=$GO_BIN version=$GO_VERSION" | tee -a mainnet-checkpoint-preparation.log
[[ "$GO_VERSION" == go1.25.7* ]] || fail "Go 1.25.7 required; found $GO_VERSION"

rm -rf "$WORK"
mkdir -m 0700 "$WORK" "$IDENTITIES"
cd "$ROOT"
go test ./internal/mainnetidentity ./cmd/hvm-mainnet-identity ./cmd/hvm-mainnet-bootstrap
go build -trimpath -o "$WORK/hvm-mainnet-identity" ./cmd/hvm-mainnet-identity
go build -trimpath -o "$WORK/hvm-mainnet-bootstrap" ./cmd/hvm-mainnet-bootstrap
chmod 0700 "$WORK"/hvm-mainnet-*

declare -A NODE_IDS=(
 [77.90.188.153]=hvm-mainnet-validator-01
 [77.90.188.154]=hvm-mainnet-validator-02
 [77.90.188.155]=hvm-mainnet-validator-03
 [77.90.188.157]=hvm-mainnet-validator-04
)
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
SCP=(scp -q -o IdentitiesOnly=yes -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
for ip in "${HOSTS[@]}"; do
 p="$IDENTITIES/$ip.public.json"
 "${SCP[@]}" "root@$ip:/etc/hashburst-hvm-mainnet/identity/public.json" "$p"
 chmod 0600 "$p"
 "$WORK/hvm-mainnet-identity" verify --protocol "$DEPLOY/protocol.freeze.json" --public "$p"
 echo "CHECKPOINT_IDENTITY_VERIFIED ip=$ip sha256=$(sha256sum "$p" | awk '{print $1}')"
done

mapfile -t PUBS < <(find "$IDENTITIES" -maxdepth 1 -name '*.public.json' -type f | sort)
[[ "${#PUBS[@]}" -eq 4 ]] || fail "expected four identities"
"$WORK/hvm-mainnet-identity" funding-plan --protocol "$DEPLOY/protocol.freeze.json" "${PUBS[@]}" > mainnet-funding-plan.json
python3 - mainnet-funding-plan.json <<'PY'
import json,sys
p=json.load(open(sys.argv[1]))
if not isinstance(p,dict): raise SystemExit("funding plan must be object")
print("HVM_MAINNET_FUNDING_PLAN_PREPARED_NO_SIGNATURE_NO_STATE_CHANGED")
PY
cp "$DEPLOY/protocol.freeze.json" mainnet-founder-signing-protocol.json
for p in "${PUBS[@]}"; do cp "$p" "mainnet-founder-$(basename "$p")"; done
sha256sum mainnet-funding-plan.json mainnet-founder-signing-protocol.json mainnet-founder-*.public.json > mainnet-founder-signing-SHA256SUMS.log
echo "FOUNDER_SIGNING_PACKAGE_PUBLIC_ONLY_PRIVATE_KEY_NOT_REQUESTED" | tee -a mainnet-checkpoint-preparation.log

[[ -f "$SOURCE/blockchain.dat" && -f "$SOURCE/blockchain.idx" ]] || fail "missing offline legacy terminal generation under $SOURCE"
[[ "$(sha256sum "$SOURCE/blockchain.dat" | awk '{print $1}')" == "d6eda91edad1b4858a1b3ddd97347f468aab998168ec818eefa67d5becabc06b" ]] || fail "terminal blockchain.dat digest mismatch"
[[ "$(sha256sum "$SOURCE/blockchain.idx" | awk '{print $1}')" == "e2ff5d9437fe2f789197e3fe5a8863ed25cc5f5a5c3ee17c5218ee511508d68f" ]] || fail "terminal blockchain.idx digest mismatch"
if [[ -f "$FREEZE" ]] && [[ "$(sha256sum "$FREEZE" | awk '{print $1}')" == "$EXPECTED_FREEZE" ]]; then
  echo "CHECKPOINT_LEGACY_EVIDENCE_PRESENT_HASH_PINNED"
  REATTEST_ARG=()
else
  [[ -f "$REATTEST" ]] || fail "original freeze evidence unavailable and reviewed re-attestation missing"
  echo "CHECKPOINT_LEGACY_EVIDENCE_RECOVERY_REATTESTATION_SELECTED original_sha256=$EXPECTED_FREEZE reattestation_sha256=$(sha256sum "$REATTEST" | awk '{print $1}')"
  REATTEST_ARG=(--re-attestation "$REATTEST")
fi

"$WORK/hvm-mainnet-bootstrap" --protocol "$DEPLOY/protocol.freeze.json" --freeze-report "$FREEZE" "${REATTEST_ARG[@]}" --source "$SOURCE" --out "$OUT"
test -f "$OUT/CHECKPOINT.json"
python3 - "$OUT/CHECKPOINT.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert d["activation_allowed"] is False
assert d["production_import_executed"] is False
assert d["chain_id"] == 4735489
assert d["checkpoint_height"] == 0
assert d["validators"] == []
print("HVM_MAINNET_ECONOMIC_CHECKPOINT_REPLAY_VERIFIED_NOT_ACTIVATABLE")
PY
cp "$OUT/CHECKPOINT.json" mainnet-economic-checkpoint.json
echo "HVM_MAINNET_CHECKPOINT_PREPARATION_READY_FOR_FOUNDER_SIGNATURE"
