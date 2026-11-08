#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"
EVIDENCE_ROOT=/var/lib/hashburst-actions/mainnet-evidence
R6_RUN_ID=42514933956
R6="$EVIDENCE_ROOT/founder-signed-checkpoints/run-$R6_RUN_ID"
R6_SIGNED="$EVIDENCE_ROOT/founder-signed-funding/founder-transfers.json"
EXPECTED_SIGNED_SHA=7298217d6f3a39253b48a0db4aca23953a41887d1dc97d050f5c8eeebb967029
SSH_KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
IDENTITY_DIR=/etc/hashburst-hvm-mainnet/identity

fail(){ echo "R7_RUNTIME_PREFLIGHT_FAIL: $*" | tee -a mainnet-runtime-acceptance.log >&2; exit 1; }
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$KNOWN" && -S /run/hashburst-control/ssh-agent.sock ]] || fail "controlled SSH unavailable"
[[ -d "$R6/ledger" && -f "$R6/CHECKPOINT.json" ]] || fail "accepted R6 checkpoint missing"
[[ -f "$R6_SIGNED" && ! -L "$R6_SIGNED" ]] || fail "signed funding input missing"
[[ "$(sha256sum "$R6_SIGNED" | awk '{print $1}')" == "$EXPECTED_SIGNED_SHA" ]] || fail "signed funding digest changed"

python3 "$DEPLOY/check-freeze.py" | tee mainnet-runtime-freeze.log
python3 - "$R6/CHECKPOINT.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert d["schema"]=="hashburst-mainnet-validator-checkpoint-v1"
assert d["activation_allowed"] is False
assert d["runtime_acceptance_required"] is True
assert d["chain_id"]==4735489
assert d["bootstrap_issuance_units"]==0
assert len(d["validator_set"])==4
print("HVM_R7_R6_CHECKPOINT_BOUNDARY_VERIFIED")
PY

cd "$ROOT"
[[ "$(go env GOVERSION)" == go1.25.7* ]] || fail "Go 1.25.7 required"
go test ./internal/testnet ./blockchain -run 'TestStatePinLockAndRestart|TestRejectMissingCorruptStateAndJournals|TestPersistentObserverLifecycle|TestMainnetBootstrap|TestMainnetImport' -count=1
go build -trimpath -o "$REPO_ROOT/hashburst-mainnet-r7" ./cmd/hashburst-mainnet
chmod 0700 "$REPO_ROOT/hashburst-mainnet-r7"
sha256sum "$REPO_ROOT/hashburst-mainnet-r7" | tee "$REPO_ROOT/mainnet-runtime-binary-SHA256.log"

SSH=(ssh -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
for ip in "${HOSTS[@]}"; do
  echo "R7_HOST_PREFLIGHT_BEGIN ip=$ip"
  SSH_AUTH_SOCK=/run/hashburst-control/ssh-agent.sock "${SSH[@]}" "root@$ip" "set -euo pipefail
    test -d '$IDENTITY_DIR'
    test ! -L '$IDENTITY_DIR'
    test -f '$IDENTITY_DIR/public.json'
    test -f '$IDENTITY_DIR/p2p.key'
    test -f '$IDENTITY_DIR/consensus.key'
    test "\$(stat -c '%a' '$IDENTITY_DIR')" = 700
    test "\$(stat -c '%a' '$IDENTITY_DIR/p2p.key')" = 600
    test "\$(stat -c '%a' '$IDENTITY_DIR/consensus.key')" = 600
    test ! -L /var/lib/hashburst-hvm-mainnet 2>/dev/null || exit 41
    if systemctl list-unit-files --no-legend 2>/dev/null | awk '{print \$1}' | grep -Fxq hashburst-hvm-mainnet.service; then
      test "\$(systemctl is-active hashburst-hvm-mainnet.service 2>/dev/null || true)" != active
    fi
    echo R7_HOST_NO_MAINNET_SERVICE_STARTED_OK
  "
  echo "R7_HOST_PREFLIGHT_OK ip=$ip"
done

rm -f "$REPO_ROOT/hashburst-mainnet-r7"
{
 echo "r6_run_id=$R6_RUN_ID"
 echo "r6_signed_funding_sha256=$EXPECTED_SIGNED_SHA"
 echo "source_commit=$GITHUB_SHA"
 echo "NO_SERVICE_STARTED"
 echo "NO_PRODUCTION_ACTIVATION"
 echo "HVM_R7A_RUNTIME_CRASH_RECOVERY_PREFLIGHT_OK"
 echo "HVM_R7B_CONTROLLED_PROVISION_RESTART_REQUIRED"
} | tee -a "$REPO_ROOT/mainnet-runtime-acceptance.log"
