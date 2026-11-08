#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"
EVIDENCE_ROOT=/var/lib/hashburst-actions/mainnet-evidence
R6_RUN_ID=42514933956
R6="$EVIDENCE_ROOT/founder-signed-checkpoints/run-$R6_RUN_ID"
R5_IDENTITIES="$REPO_ROOT/mainnet-checkpoint-preparation/identities"
KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
SERVICE=hashburst-hvm-mainnet.service
REMOTE_USER=hashburst-hvm-mainnet
STATE=/var/lib/hashburst-hvm-mainnet
ETC=/etc/hashburst-hvm-mainnet
OPT=/opt/hashburst-hvm-mainnet
CONFIGS="$REPO_ROOT/mainnet-r7b-configs"
BINARY="$REPO_ROOT/hashburst-mainnet-r7b"
LOG="$REPO_ROOT/mainnet-r7b-runtime.log"
SSH=(ssh -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
SCP=(scp -q -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)

fail(){ echo "HVM_R7B_FAIL: $*" | tee -a "$LOG" >&2; exit 1; }
remote(){ local ip=$1; shift; SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$ip" "$@"; }
copy(){ local src=$1 ip=$2 dst=$3; SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" "$src" "root@$ip:$dst"; }

[[ "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" == 77.90.188.180 ]] || fail "wrong coordinator"
[[ -S "$SOCK" && -r "$KNOWN" ]] || fail "controlled SSH unavailable"
[[ -d "$R6/ledger" && -f "$R6/CHECKPOINT.json" ]] || fail "accepted R6 checkpoint missing"
[[ -d "$R5_IDENTITIES" ]] || fail "accepted public identities missing"
python3 "$DEPLOY/check-freeze.py" | tee "$REPO_ROOT/mainnet-r7b-freeze.log"

cd "$ROOT"
[[ "$(go env GOVERSION)" == go1.25.7* ]] || fail "Go 1.25.7 required"
go test ./internal/testnet ./blockchain -run 'TestStatePinLockAndRestart|TestRejectMissingCorruptStateAndJournals|TestPersistentObserverLifecycle|TestMainnetBootstrap|TestMainnetImport' -count=1
go build -trimpath -o "$BINARY" ./cmd/hashburst-mainnet
chmod 0700 "$BINARY"
BINARY_SHA=$(sha256sum "$BINARY" | awk '{print $1}')
LEDGER_DAT_SHA=$(sha256sum "$R6/ledger/blockchain.dat" | awk '{print $1}')
LEDGER_IDX_SHA=$(sha256sum "$R6/ledger/blockchain.idx" | awk '{print $1}')
printf 'binary_sha256=%s\nledger_dat_sha256=%s\nledger_idx_sha256=%s\n' "$BINARY_SHA" "$LEDGER_DAT_SHA" "$LEDGER_IDX_SHA" | tee "$REPO_ROOT/mainnet-r7b-SHA256SUMS.log"

rm -rf "$CONFIGS"
mkdir -m 0700 "$CONFIGS"
python3 - "$DEPLOY/protocol.freeze.json" "$R6/CHECKPOINT.json" "$R5_IDENTITIES" "$CONFIGS" <<'PY'
import base64,json,pathlib,sys
protocol=json.load(open(sys.argv[1])); cp=json.load(open(sys.argv[2]))
idir=pathlib.Path(sys.argv[3]); out=pathlib.Path(sys.argv[4])
hosts={"hvm-mainnet-validator-01":"77.90.188.153","hvm-mainnet-validator-02":"77.90.188.154","hvm-mainnet-validator-03":"77.90.188.155","hvm-mainnet-validator-04":"77.90.188.157"}
vals={v["node_id"]:v for v in cp["validator_set"]["validators"]}
assert set(vals)==set(hosts)
pubs={}
for p in idir.glob("*.public.json"):
 d=json.load(open(p)); reg=d["registration"]; r=json.loads(reg["Data"])
 pubs[r["node_id"]]=(d,r)
assert set(pubs)==set(hosts)
boot=[]
for node in sorted(hosts):
 d,r=pubs[node]; v=vals[node]
 assert r["peer_id"]==v["peer_id"]
 ep=r["multiaddrs"][0]
 boot.append(ep)
for node,ip in hosts.items():
 d,r=pubs[node]; v=vals[node]
 cfg={"schema":1,"network":"mainnet","node_id":node,"role":"validator",
 "data_dir":"/var/lib/hashburst-hvm-mainnet","rpc_listen":"127.0.0.1:18019",
 "p2p_listen_ip":ip,"p2p_port":31317,"peer_id":v["peer_id"],
 "p2p_key_file":"/etc/hashburst-hvm-mainnet/p2p.key","validator_id":v["id"],
 "consensus_key_file":"/etc/hashburst-hvm-mainnet/consensus.key",
 "genesis_hash":cp["genesis_hash"],"checkpoint_height":cp["checkpoint_height"],
 "checkpoint_hash":cp["checkpoint_hash"],"bootnodes":boot,"protocol":protocol}
 (out/(ip+".json")).write_text(json.dumps(cfg,indent=2)+"\n")
print("HVM_R7B_CONFIGS_BOUND_TO_ACCEPTED_R6_4OF4_OK")
PY

# Barrier 1: all four hosts must be clean before the first mutation.
for ip in "${HOSTS[@]}"; do
  remote "$ip" "set -euo pipefail
    test -d '$ETC/identity' && test ! -L '$ETC/identity'
    test -f '$ETC/identity/public.json' && test -f '$ETC/identity/p2p.key' && test -f '$ETC/identity/consensus.key'
    test ! -e '$STATE'
    test ! -e '$OPT'
    test ! -e '$ETC/node.json'
    ! systemctl is-active --quiet '$SERVICE'
    ! ss -lnt | awk '{print \$4}' | grep -Eq '(:18019|:31317)$'
    command -v python3 >/dev/null
    command -v curl >/dev/null
    tep=\$(curl --fail --silent --show-error --max-time 5 --noproxy '*' http://127.0.0.1:47778/)
    python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("app_ready") is True; k=d.get("pubkey"); assert isinstance(k,str) and len(k)==64' <<<\"\$tep\"
    echo R7B_HOST_CLEAN_PREFLIGHT_OK ip=$ip"
done
echo HVM_R7B_MUTATION_BARRIER_4OF4_OK | tee -a "$LOG"

# Stage the exact accepted generation, reviewed binary/config and local runtime key copies.
for ip in "${HOSTS[@]}"; do
  stage="/var/lib/.hashburst-hvm-mainnet-r7b-stage"
  remote "$ip" "set -euo pipefail
    getent passwd '$REMOTE_USER' >/dev/null || useradd --system --home '$STATE' --shell /usr/sbin/nologin '$REMOTE_USER'
    install -d -m 0700 -o '$REMOTE_USER' -g '$REMOTE_USER' '$stage'
    install -d -m 0755 -o root -g root '$OPT'
    install -d -m 0755 -o root -g root '$ETC'"
  tar -C "$R6/ledger" -cf - . | SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$ip" "tar -C '$stage' -xf -"
  copy "$BINARY" "$ip" "$OPT/hashburst-mainnet"
  copy "$CONFIGS/$ip.json" "$ip" "$ETC/node.json"
  copy "$DEPLOY/hashburst-hvm-mainnet.service" "$ip" "/etc/systemd/system/$SERVICE"
  SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$ip" bash -s -- \
    "$stage" "$STATE" "$OPT" "$ETC" "$REMOTE_USER" "$SERVICE" \
    "$LEDGER_DAT_SHA" "$LEDGER_IDX_SHA" "$BINARY_SHA" "$ip" <<'REMOTE'
set -euo pipefail
stage=$1; state=$2; opt=$3; etc=$4; runtime_user=$5
service=$6; ledger_dat_sha=$7; ledger_idx_sha=$8; binary_sha=$9; ip=${10}
test "$(sha256sum "$stage/blockchain.dat" | awk '{print $1}')" = "$ledger_dat_sha"
test "$(sha256sum "$stage/blockchain.idx" | awk '{print $1}')" = "$ledger_idx_sha"
test "$(sha256sum "$opt/hashburst-mainnet" | awk '{print $1}')" = "$binary_sha"
install -m 0600 -o "$runtime_user" -g "$runtime_user" "$etc/identity/p2p.key" "$etc/p2p.key"
install -m 0600 -o "$runtime_user" -g "$runtime_user" "$etc/identity/consensus.key" "$etc/consensus.key"
chown -R "$runtime_user:$runtime_user" "$stage"
chmod 0700 "$stage"
chmod 0755 "$opt/hashburst-mainnet"
chown root:root "$opt/hashburst-mainnet" "$etc/node.json" "/etc/systemd/system/$service"
chmod 0644 "$etc/node.json" "/etc/systemd/system/$service"
mv "$stage" "$state"
systemctl daemon-reload
echo "R7B_HOST_STAGE_OK ip=$ip"
REMOTE
done
echo HVM_R7B_STAGE_4OF4_OK | tee -a "$LOG"

# Provision and replay-check all four before any listener starts.
for ip in "${HOSTS[@]}"; do
  remote "$ip" "set -euo pipefail
    runuser -u '$REMOTE_USER' -- '$OPT/hashburst-mainnet' --config '$ETC/node.json' --provision
    runuser -u '$REMOTE_USER' -- '$OPT/hashburst-mainnet' --config '$ETC/node.json' --check
    test -f '$STATE/runtime.pin'
    test -f '$STATE/consensus-votes.jsonl'
    test -f '$STATE/consensus-bft-signatures.jsonl'
    echo R7B_HOST_PROVISION_CHECK_OK ip=$ip"
done
echo HVM_R7B_PROVISION_CHECK_4OF4_OK | tee -a "$LOG"

# Controlled temporary runtime start. This is acceptance testing, not final activation.
for ip in "${HOSTS[@]}"; do remote "$ip" "systemctl start '$SERVICE'"; done

health_py='import json,sys; d=json.load(sys.stdin); assert d["ok"] is True and d["network"]=="mainnet" and d["chain_id"]==4735489 and d["reactor_running"] is True; print(d["height"],d["finalized_height"],d["peer_count"])'
for attempt in $(seq 1 60); do
  ok=1
  for ip in "${HOSTS[@]}"; do
    remote "$ip" "curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'" >/dev/null || ok=0
  done
  [[ "$ok" == 1 ]] && break
  sleep 2
done
for ip in "${HOSTS[@]}"; do
  remote "$ip" "curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'"
  echo "R7B_HOST_RUNTIME_HEALTH_OK ip=$ip" | tee -a "$LOG"
done

# Graceful restart each validator while the other three remain available.
for ip in "${HOSTS[@]}"; do
  remote "$ip" "systemctl restart '$SERVICE'"
  for attempt in $(seq 1 30); do
    remote "$ip" "curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'" >/dev/null 2>&1 && break
    sleep 2
  done
  remote "$ip" "systemctl is-active --quiet '$SERVICE'"
  echo "R7B_GRACEFUL_RESTART_OK ip=$ip" | tee -a "$LOG"
done
echo HVM_R7B_GRACEFUL_RESTART_4OF4_OK | tee -a "$LOG"

# Crash one validator at a time; systemd must restart it and the remaining quorum stays healthy.
for ip in "${HOSTS[@]}"; do
  remote "$ip" "systemctl kill -s KILL --kill-who=main '$SERVICE'"
  sleep 1
  for attempt in $(seq 1 30); do
    remote "$ip" "systemctl is-active --quiet '$SERVICE' && curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'" >/dev/null 2>&1 && break
    sleep 2
  done
  for other in "${HOSTS[@]}"; do
    [[ "$other" == "$ip" ]] && continue
    remote "$other" "curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'" >/dev/null
  done
  echo "R7B_CRASH_RECOVERY_OK ip=$ip" | tee -a "$LOG"
done
echo HVM_R7B_CRASH_RECOVERY_4OF4_OK | tee -a "$LOG"

# Wait for common BFT finality beyond the bootstrap checkpoint.
CHECKPOINT_HEIGHT=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["checkpoint_height"])' "$R6/CHECKPOINT.json")
TARGET=$((CHECKPOINT_HEIGHT+2))
for attempt in $(seq 1 90); do
  min=999999999
  peers_ok=1
  for ip in "${HOSTS[@]}"; do
    read -r h f p < <(remote "$ip" "curl -fsS http://127.0.0.1:18019/health | python3 -c '$health_py'")
    (( f < min )) && min=$f
    (( p >= 3 )) || peers_ok=0
  done
  (( min >= TARGET && peers_ok == 1 )) && break
  sleep 2
done
(( min >= TARGET && peers_ok == 1 )) || fail "fleet did not reach common finality/peer quorum"
FINALIZED=$min

commitment_py='import json,sys; d=json.load(sys.stdin); assert "error" not in d; r=d["result"]; print(json.dumps({k:r[k] for k in ("chain_id","height","hash","hbt_state_root","hvm_state_root","validator_set_root")},sort_keys=True,separators=(",",":")))'
REF=""
for ip in "${HOSTS[@]}"; do
  C=$(remote "$ip" "curl -fsS -H 'Content-Type: application/json' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"hb_getFinalizedCommitment\",\"params\":[$FINALIZED]}' http://127.0.0.1:18019/rpc | python3 -c '$commitment_py'")
  [[ -z "$REF" ]] && REF="$C"
  [[ "$C" == "$REF" ]] || fail "finalized commitment divergence at height $FINALIZED"
  echo "R7B_FINALIZED_COMMITMENT_MATCH ip=$ip height=$FINALIZED" | tee -a "$LOG"
done
echo "HVM_R7B_CONVERGENCE_4OF4_OK height=$FINALIZED commitment=$REF" | tee -a "$LOG"

# Acceptance ends stopped and disabled. State/journals remain for evidence and later reviewed activation.
for ip in "${HOSTS[@]}"; do
  SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$ip" bash -s -- \
    "$SERVICE" "$REMOTE_USER" "$OPT" "$ETC" "$ip" <<'REMOTE'
set -euo pipefail
service=$1; runtime_user=$2; opt=$3; etc=$4; ip=$5
systemctl stop "$service"
systemctl disable "$service" >/dev/null 2>&1 || true
test "$(systemctl is-active "$service" 2>/dev/null || true)" != active
runuser -u "$runtime_user" -- "$opt/hashburst-mainnet" --config "$etc/node.json" --check
echo "R7B_HOST_STOPPED_POST_ACCEPTANCE_OK ip=$ip"
REMOTE
done

{
 echo "source_commit=$GITHUB_SHA"
 echo "r6_run_id=$R6_RUN_ID"
 echo "binary_sha256=$BINARY_SHA"
 echo "accepted_finalized_height=$FINALIZED"
 echo "RUNTIME_STOPPED_AFTER_ACCEPTANCE"
 echo "NO_FINAL_PRODUCTION_ACTIVATION"
 echo "FOUNDER_PRIVATE_KEY_NOT_USED"
 echo "HVM_R7B_MAINNET_RUNTIME_CRASH_RECOVERY_ACCEPTANCE_OK"
} | tee -a "$LOG"
