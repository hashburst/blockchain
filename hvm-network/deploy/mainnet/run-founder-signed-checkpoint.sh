#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"
EVIDENCE_ROOT=/var/lib/hashburst-actions/mainnet-evidence
SIGNED_DIR="$EVIDENCE_ROOT/founder-signed-funding"
SIGNED="$SIGNED_DIR/founder-transfers.json"
R5_WORK="$REPO_ROOT/mainnet-checkpoint-preparation"
R5_OUT="$R5_WORK/economic-checkpoint"
IDENTITIES="$R5_WORK/identities"
RUN_ID="${GITHUB_RUN_ID:-}"
fail(){ echo "FOUNDER_CHECKPOINT_GATE_FAIL: $*" | tee -a mainnet-founder-checkpoint.log >&2; exit 1; }
on_err(){ rc=$?; echo "FOUNDER_CHECKPOINT_UNEXPECTED_EXIT rc=$rc line=$1 command=$2" | tee -a mainnet-founder-checkpoint.log >&2; exit "$rc"; }
trap 'on_err "$LINENO" "$BASH_COMMAND"' ERR

[[ -n "$RUN_ID" && "$RUN_ID" =~ ^[0-9]+$ ]] || fail "GitHub run id required"
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
python3 "$DEPLOY/check-freeze.py" | tee mainnet-founder-checkpoint-freeze.log

# Reproduce the accepted R5 preparation from frozen inputs before consuming any
# founder authorization. This does not sign, activate, provision or start services.
test -x "$DEPLOY/run-checkpoint-preparation.sh"
bash "$DEPLOY/run-checkpoint-preparation.sh"

[[ -d "$R5_OUT/ledger" && -f "$R5_OUT/CHECKPOINT.json" ]] || fail "R5 economic checkpoint unavailable"
[[ -d "$IDENTITIES" ]] || fail "R5 public identities unavailable"
mapfile -t PUBS < <(find "$IDENTITIES" -maxdepth 1 -name '*.public.json' -type f | sort)
[[ "${#PUBS[@]}" -eq 4 ]] || fail "expected four R5 public identities"

# founder-transfers.json is public signed data, but its import path is fixed and
# local. Never accept it from an environment variable, GitHub secret or URL.
[[ -f "$SIGNED" && ! -L "$SIGNED" ]] || fail "staged founder-transfers.json missing or not a regular non-symlink file"
[[ "$(readlink -f "$SIGNED")" == "$SIGNED" ]] || fail "founder-transfers.json path must be canonical"
mode="$(stat -c '%a' "$SIGNED")"
perm=$((8#$mode))
(( (perm & 8#022) == 0 )) || fail "founder-transfers.json must not be group/world writable"
size="$(stat -c '%s' "$SIGNED")"
(( size > 0 && size <= 1048576 )) || fail "founder-transfers.json size invalid"
SIGNED_SHA256="$(sha256sum "$SIGNED" | awk '{print $1}')"
echo "HVM_R6_SIGNED_FUNDING_STAGED sha256=$SIGNED_SHA256" | tee -a mainnet-founder-checkpoint.log

cd "$ROOT"
GO_BIN="$(command -v go || true)"
[[ -n "$GO_BIN" ]] || fail "Go unavailable"
GO_VERSION="$("$GO_BIN" env GOVERSION 2>/dev/null || true)"
[[ "$GO_VERSION" == go1.25.7* ]] || fail "Go 1.25.7 required; found $GO_VERSION"
go test ./internal/mainnetidentity ./cmd/hvm-mainnet-identity
go build -trimpath -o "$R5_WORK/hvm-mainnet-identity-r6" ./cmd/hvm-mainnet-identity
chmod 0700 "$R5_WORK/hvm-mainnet-identity-r6"

"$R5_WORK/hvm-mainnet-identity-r6" verify-funding \
  --protocol "$DEPLOY/protocol.freeze.json" \
  --signed-funding "$SIGNED" "${PUBS[@]}" | tee -a "$REPO_ROOT/mainnet-founder-checkpoint.log"
echo "HVM_R6_FOUNDER_FUNDING_VERIFIED_NO_STATE_CHANGED" | tee -a "$REPO_ROOT/mainnet-founder-checkpoint.log"

R6_PARENT="$EVIDENCE_ROOT/founder-signed-checkpoints"
mkdir -p "$R6_PARENT"
[[ "$(readlink -f "$R6_PARENT")" == "$R6_PARENT" ]] || fail "R6 evidence parent must be canonical"
OUT="$R6_PARENT/run-$RUN_ID"
[[ ! -e "$OUT" ]] || fail "R6 output already exists for this run"

"$R5_WORK/hvm-mainnet-identity-r6" assemble \
  --protocol "$DEPLOY/protocol.freeze.json" \
  --economic-ledger "$R5_OUT/ledger" \
  --signed-funding "$SIGNED" \
  --out "$OUT" "${PUBS[@]}" | tee -a "$REPO_ROOT/mainnet-founder-checkpoint.log"

[[ -f "$OUT/CHECKPOINT.json" && -d "$OUT/ledger" ]] || fail "validator checkpoint incomplete"
python3 - "$OUT/CHECKPOINT.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert d["schema"] == "hashburst-mainnet-validator-checkpoint-v1"
assert d["activation_allowed"] is False
assert d["runtime_acceptance_required"] is True
assert d["chain_id"] == 4735489
assert d["bootstrap_issuance_units"] == 0
print("HVM_R6_VALIDATOR_CHECKPOINT_POLICY_VERIFIED_NOT_ACTIVATABLE")
PY

cd "$REPO_ROOT"
cp "$SIGNED" mainnet-founder-transfers.json
cp "$OUT/CHECKPOINT.json" mainnet-validator-checkpoint.json
sha256sum \
  mainnet-founder-transfers.json \
  mainnet-validator-checkpoint.json \
  "$OUT/ledger/blockchain.dat" \
  "$OUT/ledger/blockchain.idx" > mainnet-founder-checkpoint-SHA256SUMS.log
{
  echo "source_commit=$GITHUB_SHA"
  echo "github_run_id=$RUN_ID"
  echo "signed_funding_sha256=$SIGNED_SHA256"
  echo "validator_checkpoint_dir=$OUT"
  echo "NO_SERVICE_STARTED"
  echo "NO_PRODUCTION_ACTIVATION"
  echo "HVM_R6_FOUNDER_SIGNED_FUNDING_AND_OFFLINE_VALIDATOR_CHECKPOINT_OK"
} | tee -a mainnet-founder-checkpoint.log
