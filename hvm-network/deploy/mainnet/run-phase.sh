#!/usr/bin/env bash
set -euo pipefail

PHASE="${1:-}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
SSH_KEY=/etc/hashburst-control/fleet_ed25519
KNOWN=/etc/hashburst-control/fleet_known_hosts
OUT="$ROOT/../mainnet-identities"
REMOTE_ROOT=/var/tmp/hashburst-hvm-mainnet-identity-gate
IDENTITY_DIR=/etc/hashburst-hvm-mainnet/identity
P2P_PORT=31317

fail(){ echo "IDENTITIES_GATE_FAIL: $*" >&2; exit 1; }
[[ "$PHASE" == identities ]] || fail "only identities is implemented; checkpoint/deploy/acceptance remain fail-closed"
[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -r "$SSH_KEY" && -r "$KNOWN" ]] || fail "fleet SSH material unavailable"
python3 "$DEPLOY/check-freeze.py" | tee mainnet-identities-freeze.log
rm -rf "$OUT"
mkdir -m 0700 "$OUT"

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
  "${SSH[@]}" "root@$ip" "test ! -e '$IDENTITY_DIR' && test ! -L '$IDENTITY_DIR' && mkdir -p '$REMOTE_ROOT'"
  "${SCP[@]}" "$DEPLOY/protocol.freeze.json" "root@$ip:$REMOTE_ROOT/protocol.json"
  tar -C "$ROOT" -cf - go.mod go.sum cmd internal blockchain consensus protocolv2 wallet |     "${SSH[@]}" "root@$ip" "set -e; rm -rf '$REMOTE_ROOT/src'; mkdir -m 0700 '$REMOTE_ROOT/src'; tar -C '$REMOTE_ROOT/src' -xf -"
  "${SSH[@]}" "root@$ip" "set -euo pipefail
    cd '$REMOTE_ROOT/src'
    go test ./internal/mainnetidentity ./cmd/hvm-mainnet-identity
    go build -trimpath -o '$REMOTE_ROOT/hvm-mainnet-identity' ./cmd/hvm-mainnet-identity
    tep_json=\$(curl --fail --silent --show-error --max-time 5 --noproxy '*' http://127.0.0.1:47778/)
    tep_key=\$(python3 -c 'import json,sys; d=json.load(sys.stdin); k=d.get("pubkey",""); assert d.get("app_ready") is True and isinstance(k,str) and len(k)==64 and len(bytes.fromhex(k))==32 and any(bytes.fromhex(k)); print(k.lower())' <<<\"\$tep_json\")
    '$REMOTE_ROOT/hvm-mainnet-identity' generate --protocol '$REMOTE_ROOT/protocol.json' --out '$IDENTITY_DIR' --node-id '$node' --ip '$ip' --p2p-port '$P2P_PORT' --tep-public-key \$tep_key
    '$REMOTE_ROOT/hvm-mainnet-identity' verify --protocol '$REMOTE_ROOT/protocol.json' --public '$IDENTITY_DIR/public.json'
    test \"\$(stat -c '%a' '$IDENTITY_DIR')\" = 700
    for f in p2p.key consensus.key operator.key reward.key public.json; do test \"\$(stat -c '%a' '$IDENTITY_DIR/'\$f)\" = 600; done
    sha256sum '$IDENTITY_DIR/public.json' | awk '{print \$1}'
  " > "$OUT/$ip.remote.log"
  "${SCP[@]}" "root@$ip:$IDENTITY_DIR/public.json" "$OUT/$ip.public.json"
  chmod 0600 "$OUT/$ip.public.json"
  echo "IDENTITY_PUBLIC_EXPORT_OK ip=$ip sha256=$(sha256sum "$OUT/$ip.public.json" | awk '{print $1}')"
done

cd "$ROOT"
go build -trimpath -o "$OUT/hvm-mainnet-identity" ./cmd/hvm-mainnet-identity
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
 r=json.loads(d["registration"]["data"])
 q=json.loads(d["validator_registration"]["data"])
 vals={
  "node":r["node_id"],"operator":d["registration"]["sender"],"peer":r["peer_id"],
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
