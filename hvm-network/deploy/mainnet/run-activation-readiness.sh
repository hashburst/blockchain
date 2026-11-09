#!/usr/bin/env bash
set -euo pipefail

# HVM-R7C is deliberately read-only. It binds the accepted R7B evidence to the
# exact source/runtime and proves the four validators are stopped, disabled and
# still carry the accepted runtime/state boundary. It does NOT activate mainnet.
KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock
SERVICE=hashburst-hvm-mainnet.service
BIN=/opt/hashburst-hvm-mainnet/hashburst-mainnet
STATE=/var/lib/hashburst-hvm-mainnet
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)

R7B_RUN_ID=42616952745
R7B_RUN_NUMBER=51
R7B_SOURCE_SHA=c07ffa33740fb369dafccfce680e86b8554bb1b4
R7B_ARTIFACT_ID=13673827024
R7B_ARTIFACT_SHA256=a97f82f0359ce805246c94cad27e5d136965eeabe765f07d94ef796fc738a543
R7B_BINARY_SHA256=cf111db0d141e76787b76cc319f97fa6a4608de2868d4ba0d94381ba853fb2b8
R7B_ACCEPTED_FINALIZED_HEIGHT=29
R7B_FINALIZED_HASH=dcaf4cf790cfca78bc9ac2cefa0b93e6e8264685041a0bb6a8df7e7f4ca6b844
R7B_HBT_STATE_ROOT=32dfaaa98bc58d71668fbf7078e8ad0e25ec71208ec7fd06786d52ba27d1b30f
R7B_HVM_STATE_ROOT=ba2eae8aa315f0566035e0adaeab85d80d7e5648b49732194290d1333a34a4ca
R7B_VALIDATOR_SET_ROOT=0x85be73919c61085d593c4ac64df3631896f85a3f82a664c0e433d87f57abef52

fail(){ echo "HVM_R7C_FAIL: $*" >&2; exit 1; }
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$KNOWN" && -S "$SOCK" ]] || fail "control-plane SSH boundary unavailable"
git cat-file -e "$R7B_SOURCE_SHA^{commit}" || fail "accepted R7B source commit unavailable"
git merge-base --is-ancestor "$R7B_SOURCE_SHA" "$GITHUB_SHA" || fail "accepted R7B source is not an ancestor of this reviewed gate"
echo "R7C_SOURCE_LINEAGE_OK accepted_r7b_source=$R7B_SOURCE_SHA gate_source=$GITHUB_SHA"

export SSH_AUTH_SOCK="$SOCK"
SSH=(ssh -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
for ip in "${HOSTS[@]}"; do
  echo "R7C_AT_REST_CHECK_BEGIN ip=$ip"
  "${SSH[@]}" "root@$ip" "set -euo pipefail
    test \"\$(systemctl is-active '$SERVICE' || true)\" = inactive
    test \"\$(systemctl is-enabled '$SERVICE' || true)\" = disabled
    test -f '$BIN'
    test \"\$(sha256sum '$BIN' | awk '{print \$1}')\" = '$R7B_BINARY_SHA256'
    test -f '$STATE/runtime.pin'
    test -f '$STATE/consensus-votes.jsonl'
    test -f '$STATE/consensus-bft-signatures.jsonl'
    test -f '$STATE/consensus-recovery.json'
    test ! -L '$STATE'
    echo R7C_AT_REST_HOST_OK
  " | tee -a "$tmp"
  echo "R7C_AT_REST_CHECK_OK ip=$ip"
done
[[ "$(grep -c '^R7C_AT_REST_HOST_OK$' "$tmp")" == 4 ]] || fail "at-rest validator count mismatch"

cat > mainnet-r7c-activation-readiness.json <<JSON
{
  "schema": "hashburst-hvm-mainnet-r7c-activation-readiness-v1",
  "chain_id": 4735489,
  "r7b": {
    "canonical": true,
    "run_id": $R7B_RUN_ID,
    "run_number": $R7B_RUN_NUMBER,
    "source_sha": "$R7B_SOURCE_SHA",
    "artifact_id": $R7B_ARTIFACT_ID,
    "artifact_sha256": "$R7B_ARTIFACT_SHA256",
    "binary_sha256": "$R7B_BINARY_SHA256",
    "accepted_finalized_height": $R7B_ACCEPTED_FINALIZED_HEIGHT,
    "finalized_hash": "$R7B_FINALIZED_HASH",
    "hbt_state_root": "$R7B_HBT_STATE_ROOT",
    "hvm_state_root": "$R7B_HVM_STATE_ROOT",
    "validator_set_root": "$R7B_VALIDATOR_SET_ROOT"
  },
  "validator_at_rest_4of4": true,
  "production_activation_authorized": false,
  "remaining_gates": [
    "persistent production APoW source and custody/service acceptance",
    "observer-backed public RPC ingress acceptance",
    "mainnet wallet transaction/receipt/log acceptance",
    "mainnet WebSocket subscription acceptance",
    "final release manifest bound to canonical R7B/R7C evidence"
  ]
}
JSON
chmod 0444 mainnet-r7c-activation-readiness.json
sha256sum mainnet-r7c-activation-readiness.json | tee mainnet-r7c-activation-readiness.log
{
  echo "HVM_R7C_CANONICAL_R7B_RUN_51_BOUND_OK"
  echo "HVM_R7C_VALIDATORS_STOPPED_DISABLED_4OF4_OK"
  echo "HVM_R7C_PRODUCTION_ACTIVATION_READINESS_EVIDENCE_OK"
  echo "NO_SERVICE_STARTED"
  echo "NO_PRODUCTION_ACTIVATION"
  echo "FOUNDER_PRIVATE_KEY_NOT_USED"
} | tee -a mainnet-r7c-activation-readiness.log
