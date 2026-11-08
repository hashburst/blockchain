#!/usr/bin/env bash
set -euo pipefail

PHASE="${1:-}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
SSH_KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
OUT="$ROOT/../mainnet-identities"
trap 'rm -f "$OUT/hvm-mainnet-identity"; rm -rf "$OUT"; rm -f mainnet-identities.json' EXIT
REMOTE_ROOT=/var/tmp/hashburst-hvm-mainnet-identity-gate
IDENTITY_DIR=/etc/hashburst-hvm-mainnet/identity
P2P_PORT=31317
LEGACY_PROTOCOL_SHA256=9979333faf55b815ac127a8a90d705982ba1658bb0801f72f7216fb3876aa7ff
IDENTITY_ARCHIVE_ROOT=/etc/hashburst-hvm-mainnet/identity-archive

fail(){ echo "IDENTITIES_GATE_FAIL: $*" >&2; exit 1; }
[[ "$PHASE" == identities ]] || fail "only identities is implemented; checkpoint/deploy/acceptance remain fail-closed"
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$SSH_KEY" && -r "$KNOWN" ]] || fail "fleet SSH material unavailable"
python3 "$DEPLOY/check-freeze.py" | tee mainnet-identities-freeze.log
rm -rf "$OUT"
mkdir -m 0700 "$OUT"
# Build the reviewed identity helper once on the controlled runner. Validators do
# not need a Go toolchain; only the resulting binary is transferred to them.
command -v go >/dev/null 2>&1 || fail "Go toolchain unavailable on coordinator"
go_ver="$(go env GOVERSION 2>/dev/null || true)"
[[ "$go_ver" == go1.25.7* ]] || fail "coordinator Go toolchain must satisfy frozen go.mod (want go1.25.7, got $go_ver)"
cd "$ROOT"
go test ./internal/mainnetidentity ./cmd/hvm-mainnet-identity
go build -trimpath -o "$OUT/hvm-mainnet-identity" ./cmd/hvm-mainnet-identity
chmod 0700 "$OUT/hvm-mainnet-identity"
PROTOCOL_COMMITMENT_SHA256="$("$OUT/hvm-mainnet-identity" commitment --protocol "$DEPLOY/protocol.freeze.json")"
[[ "$PROTOCOL_COMMITMENT_SHA256" =~ ^[0-9a-f]{64}$ ]] || fail "invalid canonical protocol commitment"
echo "HVM_MAINNET_PROTOCOL_COMMITMENT_SHA256=$PROTOCOL_COMMITMENT_SHA256"

declare -A NODE_IDS=(
  [77.90.188.153]=hvm-mainnet-validator-01
  [77.90.188.154]=hvm-mainnet-validator-02
  [77.90.188.155]=hvm-mainnet-validator-03
  [77.90.188.157]=hvm-mainnet-validator-04
)
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
SSH=(ssh -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
SCP=(scp -q -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)

for ip in "${HOSTS[@]}"; do
  node="${NODE_IDS[$ip]}"
  echo "IDENTITY_LOCAL_GENERATION_BEGIN ip=$ip node=$node"

  # Preflight every dependency that is needed after a legacy archive move.
  # This MUST complete before any canonical identity is moved.
  "${SSH[@]}" "root@$ip" "set -euo pipefail
    command -v python3 >/dev/null
    command -v curl >/dev/null
    test ! -L '$IDENTITY_DIR'
    test ! -L '$IDENTITY_ARCHIVE_ROOT'
    tep_json=''
    for attempt in 1 2 3; do
      if tep_json=\$(curl --fail --silent --show-error --max-time 5 --noproxy '*' http://127.0.0.1:47778/); then
        break
      fi
      echo \"IDENTITY_PREFLIGHT_TEP_RETRY attempt=\$attempt\" >&2
      sleep 2
    done
    if [ -z \"\$tep_json\" ]; then
      echo 'IDENTITIES_GATE_FAIL: TEP readiness endpoint unavailable after 3 attempts' >&2
      exit 1
    fi
    if ! python3 -c 'import json,sys; d=json.load(sys.stdin); k=d.get(sys.argv[1]); assert d.get(sys.argv[2]) is True and isinstance(k,str) and len(k)==64 and len(bytes.fromhex(k))==32 and any(bytes.fromhex(k))' pubkey app_ready <<<\"\$tep_json\"; then
      echo 'IDENTITIES_GATE_FAIL: TEP readiness payload invalid or app_ready is not true' >&2
      exit 1
    fi
    echo 'IDENTITY_PREFLIGHT_TEP_READY'
    mkdir -p '$REMOTE_ROOT'
  "
  "${SCP[@]}" "$DEPLOY/protocol.freeze.json" "root@$ip:$REMOTE_ROOT/protocol.json"
  "${SCP[@]}" "$OUT/hvm-mainnet-identity" "root@$ip:$REMOTE_ROOT/hvm-mainnet-identity"

  "${SSH[@]}" "root@$ip" "set -euo pipefail
    chmod 0700 '$REMOTE_ROOT/hvm-mainnet-identity'
    '$REMOTE_ROOT/hvm-mainnet-identity' verify --protocol '$REMOTE_ROOT/protocol.json' --public '$IDENTITY_ARCHIVE_ROOT/$LEGACY_PROTOCOL_SHA256/public.json' >/dev/null 2>&1 || true

    current_hash=''
    if [ -e '$IDENTITY_DIR' ]; then
      test -d '$IDENTITY_DIR'
      test -f '$IDENTITY_DIR/public.json'
      current_hash=\$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get(sys.argv[2]))' '$IDENTITY_DIR/public.json' protocol_sha256)
    fi

    archive='$IDENTITY_ARCHIVE_ROOT/$LEGACY_PROTOCOL_SHA256'
    if [ \"\$current_hash\" = '$LEGACY_PROTOCOL_SHA256' ]; then
      test ! -e \"\$archive\"
      test \"\$(stat -c '%a' '$IDENTITY_DIR')\" = 700
      for f in p2p.key consensus.key operator.key reward.key public.json; do
        test -f '$IDENTITY_DIR/'\$f
        test \"\$(stat -c '%a' '$IDENTITY_DIR/'\$f)\" = 600
      done
      mkdir -p '$IDENTITY_ARCHIVE_ROOT'
      chmod 0700 '$IDENTITY_ARCHIVE_ROOT'
      mv '$IDENTITY_DIR' \"\$archive\"
      echo 'IDENTITY_LEGACY_ARCHIVED protocol=$LEGACY_PROTOCOL_SHA256'
      current_hash=''
    elif [ -n \"\$current_hash\" ]; then
      # A canonical identity that is neither the known legacy commitment nor
      # the current frozen commitment is never overwritten.
      if [ \"\$current_hash\" != '$PROTOCOL_COMMITMENT_SHA256' ]; then
        echo \"IDENTITIES_GATE_FAIL: unknown canonical protocol commitment \$current_hash\" >&2
        exit 1
      fi
      if ! '$REMOTE_ROOT/hvm-mainnet-identity' verify --protocol '$REMOTE_ROOT/protocol.json' --public '$IDENTITY_DIR/public.json'; then
        echo 'IDENTITIES_GATE_FAIL: current canonical identity verification failed' >&2
        exit 1
      fi
      echo IDENTITY_CURRENT_ALREADY_PRESENT
    fi

    if [ ! -e '$IDENTITY_DIR' ]; then
      test -d \"\$archive\"
      test -f \"\$archive/public.json\"
      archive_hash=\$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get(sys.argv[2]))' \"\$archive/public.json\" protocol_sha256)
      test \"\$archive_hash\" = '$LEGACY_PROTOCOL_SHA256'
      test \"\$(stat -c '%a' \"\$archive\")\" = 700
      for f in p2p.key consensus.key operator.key reward.key public.json; do
        test -f \"\$archive/\$f\"
        test \"\$(stat -c '%a' \"\$archive/\$f\")\" = 600
      done
      echo 'IDENTITY_RESUME_FROM_VERIFIED_LEGACY_ARCHIVE'
      tep_json=''
      for attempt in 1 2 3; do
        if tep_json=\$(curl --fail --silent --show-error --max-time 5 --noproxy '*' http://127.0.0.1:47778/); then
          break
        fi
        echo \"IDENTITY_GENERATION_TEP_RETRY attempt=\$attempt\" >&2
        sleep 2
      done
      [ -n \"\$tep_json\" ] || { echo 'IDENTITIES_GATE_FAIL: TEP unavailable before identity generation' >&2; exit 1; }
      if ! tep_key=\$(python3 -c 'import json,sys; d=json.load(sys.stdin); k=d.get(sys.argv[1],sys.argv[3]); assert d.get(sys.argv[2]) is True and isinstance(k,str) and len(k)==64 and len(bytes.fromhex(k))==32 and any(bytes.fromhex(k)); print(k.lower())' pubkey app_ready '' <<<\"\$tep_json\"); then
        echo 'IDENTITIES_GATE_FAIL: invalid TEP readiness payload before identity generation' >&2
        exit 1
      fi
      '$REMOTE_ROOT/hvm-mainnet-identity' generate --protocol '$REMOTE_ROOT/protocol.json' --out '$IDENTITY_DIR' --node-id '$node' --ip '$ip' --p2p-port '$P2P_PORT' --tep-public-key \$tep_key
    fi

    '$REMOTE_ROOT/hvm-mainnet-identity' verify --protocol '$REMOTE_ROOT/protocol.json' --public '$IDENTITY_DIR/public.json'
    test \"\$(stat -c '%a' '$IDENTITY_DIR')\" = 700
    for f in p2p.key consensus.key operator.key reward.key public.json; do
      test \"\$(stat -c '%a' '$IDENTITY_DIR/'\$f)\" = 600
    done
  "
  "${SCP[@]}" "root@$ip:$IDENTITY_DIR/public.json" "$OUT/$ip.public.json"
  chmod 0600 "$OUT/$ip.public.json"
  grep -q '"protocol_sha256"' "$OUT/$ip.public.json" || fail "public enrollment missing protocol commitment for $ip"
  echo "IDENTITY_PUBLIC_EXPORT_OK ip=$ip sha256=$(sha256sum "$OUT/$ip.public.json" | awk '{print $1}')"
done

for ip in "${HOSTS[@]}"; do
  "$OUT/hvm-mainnet-identity" verify --protocol "$DEPLOY/protocol.freeze.json" --public "$OUT/$ip.public.json"
done

python3 - "$OUT" <<'PY'
import json, pathlib, sys
root=pathlib.Path(sys.argv[1])
files=sorted(root.glob("*.public.json"))
if len(files)!=4: raise SystemExit("expected exactly four public identities")
seen={k:set() for k in ("node","operator","peer","consensus","tep","endpoint")}
summary=[]
for p in files:
 d=json.loads(p.read_text())
 registration=d["registration"]
 validator=d["validator_registration"]
 # blockchain.Transaction is the legacy transaction type and intentionally has
 # no JSON tags: encoding/json therefore emits Data/Sender with Go field names.
 r=json.loads(registration["Data"])
 # protocolv2.TransactionV2.Data is []byte; encoding/json emits it as base64.
 import base64
 q=json.loads(base64.b64decode(validator["data"], validate=True).decode("utf-8"))
 vals={
  "node":r["node_id"],"operator":registration["Sender"],"peer":r["peer_id"],
  "consensus":q["consensus_pub_key"],"tep":r["tep_pubkey"],"endpoint":r["multiaddrs"][0],
 }
 for k,v in vals.items():
  if v in seen[k]: raise SystemExit(f"duplicate {k}: {v}")
  seen[k].add(v)
 summary.append({"file":p.name,"node_id":vals["node"],"operator":vals["operator"],
                 "peer_id":vals["peer"],"consensus_pub_key":vals["consensus"],
                 "tep_pubkey":vals["tep"],"endpoint":vals["endpoint"],
                 "reward_address":q["reward_address"]})
(root/"summary.json").write_text(json.dumps({"schema":1,"count":4,"identities":summary},indent=2)+"\n")
PY
cp "$OUT/summary.json" mainnet-identities.json
echo "HVM_MAINNET_VALIDATOR_IDENTITIES_4OF4_OK" | tee mainnet-identities.log
echo "PRIVATE_KEYS_REMAIN_ON_VALIDATORS_PUBLIC_JSON_ONLY_EXPORTED" | tee -a mainnet-identities.log
