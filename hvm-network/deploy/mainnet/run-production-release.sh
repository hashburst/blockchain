#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
DEPLOY="$ROOT/deploy/mainnet"
KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock
VALIDATORS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
OBSERVER=64.31.4.9
VSERVICE=hashburst-hvm-mainnet.service
APOW_SERVICE=hashburst-apow-mainnet.service
OBS_SERVICE=hashburst-hvm-mainnet-ingress.service
GW_SERVICE=hashburst-hvm-mainnet-evm-gateway.service
OBS_USER=hashburst-hvm-mainnet-ingress
OBS_ETC=/etc/hashburst-hvm-mainnet-ingress
OBS_STATE=/var/lib/hashburst-hvm-mainnet-ingress
OBS_OPT=/opt/hashburst-hvm-mainnet-ingress
GW_OPT=/opt/hashburst-hvm-mainnet-evm-gateway
R6=/var/lib/hashburst-actions/mainnet-evidence/founder-signed-checkpoints/run-42514933956
R8A_SOURCE=5a5a43c517dabe6aaec80c1df6d8ce0cadd0cf08
R8A_RUN=42761659413
R8A_ARTIFACT_DIGEST=2b80e4ed1b24656c9d29ebc62e7b834ee18d39c15eb4d55183793b713abc3b85
LOG="$REPO/mainnet-production-release.log"
EVIDENCE="$REPO/mainnet-production-release.json"
SSH=(ssh -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
SCP=(scp -q -o UserKnownHostsFile="$KNOWN" -o StrictHostKeyChecking=yes -o BatchMode=yes)
remote(){ local ip=$1;shift;SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" "root@$ip" "$@"; }
copy(){ local src=$1 ip=$2 dst=$3;SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" "$src" "root@$ip:$dst"; }
fail(){ echo "HVM_PRODUCTION_RELEASE_FAIL: $*" | tee -a "$LOG" >&2;exit 1; }

test "$(hostname -I | tr ' ' '\n' | grep -Fx '77.90.188.180')" = 77.90.188.180
test -S "$SOCK" && test -r "$KNOWN"
python3 "$DEPLOY/check-freeze.py" | tee "$REPO/mainnet-production-freeze.log"
git merge-base --is-ancestor "$R8A_SOURCE" "$GITHUB_SHA" || fail "R8A source is not an ancestor"
test -f "$R6/CHECKPOINT.json" && test -f "$R6/ledger/blockchain.dat" && test -f "$R6/ledger/blockchain.idx"
echo "HVM_R8A_CANONICAL_ACCEPTANCE_BOUND run=$R8A_RUN artifact_digest=$R8A_ARTIFACT_DIGEST" | tee -a "$LOG"

for ip in "${VALIDATORS[@]}";do
 remote "$ip" "set -euo pipefail; test \"\$(systemctl is-active '$VSERVICE' || true)\" != active; test \"\$(systemctl is-enabled '$VSERVICE' || true)\" != enabled; test -f /var/lib/hashburst-hvm-mainnet/runtime.pin"
done
remote "$OBSERVER" "set -euo pipefail; systemctl is-active --quiet hashburst-hvm-testnet-ingress.service; test -f /etc/hashburst-hvm-testnet-ingress/node.json"
echo HVM_R8B_VALIDATOR_AT_REST_AND_TESTNET_OBSERVER_PRESERVED_OK | tee -a "$LOG"

cd "$ROOT"
test "$(go env GOVERSION)" = go1.25.7
go test ./internal/evmgateway ./blockchain ./cmd/hvm-observer-identity -count=1
go build -trimpath -o "$REPO/hvm-mainnet-observer-identity" ./cmd/hvm-observer-identity
go build -trimpath -o "$REPO/hvm-mainnet-evm-gateway" ./cmd/hvm-mainnet-evm-gateway
go build -trimpath -o "$REPO/hvm-mainnet-production-canary" ./cmd/hvm-mainnet-production-canary
chmod 0755 "$REPO"/hvm-mainnet-{observer-identity,evm-gateway,production-canary}

TMP=$(mktemp -d)
SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" root@77.90.188.153:/opt/hashburst-hvm-mainnet/hashburst-mainnet "$TMP/hashburst-mainnet"
SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" root@77.90.188.153:/etc/hashburst-hvm-mainnet/node.json "$TMP/validator.json"
RUNTIME_SHA=$(sha256sum "$TMP/hashburst-mainnet"|awk '{print $1}')
for ip in "${VALIDATORS[@]}";do remote "$ip" "test \"\$(sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | cut -d ' ' -f1)\" = '$RUNTIME_SHA'";done
echo "HVM_R8B_ACCEPTED_RUNTIME_4OF4_OK sha256=$RUNTIME_SHA" | tee -a "$LOG"

rollback=1
cleanup(){
 rc=$?;trap - EXIT INT TERM
 if ((rollback==1 && rc!=0));then
  for ip in "${VALIDATORS[@]}";do remote "$ip" "systemctl disable --now '$VSERVICE' >/dev/null 2>&1 || true" || true;done
  remote 77.90.188.153 "systemctl disable --now '$APOW_SERVICE' >/dev/null 2>&1 || true" || true
  remote "$OBSERVER" "systemctl disable --now '$GW_SERVICE' '$OBS_SERVICE' >/dev/null 2>&1 || true; if test -f /root/.hvm-mainnet-vhost.pre-r9;then cp -f /root/.hvm-mainnet-vhost.pre-r9 /etc/nginx/sites-available/blockchainapi.one.conf;rm -f /etc/nginx/snippets/hvm-mainnet-evm.conf;nginx -t >/dev/null&&systemctl reload nginx||true;fi" || true
  echo "HVM_R9_FAIL_CLOSED_PRODUCTION_SERVICES_STOPPED rc=$rc" | tee -a "$LOG"
 fi
 rm -rf "$TMP"
 exit "$rc"
}
trap cleanup EXIT INT TERM
echo HVM_R8B_ROLLBACK_ARMED_BEFORE_OBSERVER_MUTATION_OK | tee -a "$LOG"

remote "$OBSERVER" "set -euo pipefail
 getent passwd '$OBS_USER' >/dev/null || useradd --system --user-group --no-create-home --shell /usr/sbin/nologin '$OBS_USER'
 install -d -m 0700 -o '$OBS_USER' -g '$OBS_USER' '$OBS_ETC' '$OBS_STATE'
 install -d -m 0755 -o root -g root '$OBS_OPT' '$GW_OPT'"
copy "$REPO/hvm-mainnet-observer-identity" "$OBSERVER" "$OBS_OPT/observer-identity"
remote "$OBSERVER" "chown root:root '$OBS_OPT/observer-identity';chmod 0755 '$OBS_OPT/observer-identity'; '$OBS_OPT/observer-identity' --key '$OBS_ETC/p2p.key'; chown '$OBS_USER:$OBS_USER' '$OBS_ETC/p2p.key';chmod 0600 '$OBS_ETC/p2p.key'" > "$TMP/peer"
PEER=$(tail -n1 "$TMP/peer")
python3 - "$TMP/validator.json" "$TMP/observer.json" "$PEER" <<'PY'
import json,sys
c=json.load(open(sys.argv[1]))
assert c["network"]=="mainnet" and c["protocol"]["chain_id"]==4735489
c.update(node_id="hvm-mainnet-observer-01",role="observer",peer_id=sys.argv[3],validator_id="",consensus_key_file="",data_dir="/var/lib/hashburst-hvm-mainnet-ingress",p2p_key_file="/etc/hashburst-hvm-mainnet-ingress/p2p.key",rpc_listen="127.0.0.1:18019",p2p_listen_ip="64.31.4.9",p2p_port=31317)
json.dump(c,open(sys.argv[2],"w"),indent=2);open(sys.argv[2],"a").write("\n")
PY
copy "$TMP/hashburst-mainnet" "$OBSERVER" "$OBS_OPT/hashburst-mainnet"
copy "$TMP/observer.json" "$OBSERVER" "$OBS_ETC/node.json"
tar -C "$R6/ledger" -cf - blockchain.dat blockchain.idx | SSH_AUTH_SOCK="$SOCK" "${SSH[@]}" root@"$OBSERVER" "tar -C '$OBS_STATE' -xf -"
remote "$OBSERVER" "set -euo pipefail
 chown root:root '$OBS_OPT/hashburst-mainnet' '$OBS_ETC/node.json';chmod 0755 '$OBS_OPT/hashburst-mainnet';chmod 0644 '$OBS_ETC/node.json'
 chown -R '$OBS_USER:$OBS_USER' '$OBS_STATE' '$OBS_ETC/p2p.key'
 if test -e '$OBS_STATE/runtime.pin'; then
   test -f '$OBS_STATE/runtime.pin'
   test ! -L '$OBS_STATE/runtime.pin'
   runuser -u '$OBS_USER' -- '$OBS_OPT/hashburst-mainnet' --config '$OBS_ETC/node.json' --check
   echo HVM_R8B_MAINNET_OBSERVER_EXISTING_PIN_REVALIDATED_OK
 else
   runuser -u '$OBS_USER' -- '$OBS_OPT/hashburst-mainnet' --config '$OBS_ETC/node.json' --provision
   echo HVM_R8B_MAINNET_OBSERVER_FRESH_PROVISION_OK
 fi
 cat > /etc/systemd/system/$OBS_SERVICE <<'UNIT'
[Unit]
Description=HashBurst HVM mainnet non-signing observer
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=hashburst-hvm-mainnet-ingress
Group=hashburst-hvm-mainnet-ingress
ExecStart=/opt/hashburst-hvm-mainnet-ingress/hashburst-mainnet --config /etc/hashburst-hvm-mainnet-ingress/node.json
Restart=on-failure
RestartSec=5
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
ReadWritePaths=/var/lib/hashburst-hvm-mainnet-ingress
[Install]
WantedBy=multi-user.target
UNIT
 systemctl daemon-reload
 test \"\$(systemctl is-active '$OBS_SERVICE' || true)\" != active
 test \"\$(systemctl is-enabled '$OBS_SERVICE' || true)\" != enabled
 systemctl is-active --quiet hashburst-hvm-testnet-ingress.service"
echo "HVM_R8B_MAINNET_OBSERVER_STAGED_UNSIGNED_OK peer_id=$PEER" | tee -a "$LOG"

for ip in "${VALIDATORS[@]}";do remote "$ip" "systemctl enable --now '$VSERVICE'";done
remote 77.90.188.153 "systemctl enable --now '$APOW_SERVICE'"
for n in $(seq 1 60);do
 ok=1
 for ip in "${VALIDATORS[@]}";do remote "$ip" "curl -fsS http://127.0.0.1:18019/health | python3 -c 'import json,sys;d=json.load(sys.stdin);assert d[\"ok\"] and d[\"chain_id\"]==4735489 and d[\"peer_count\"]>=3'" >/dev/null||ok=0;done
 ((ok==1))&&break;sleep 2
done
((ok==1))||fail "validator fleet health/quorum timeout"
echo HVM_R9_VALIDATOR_FEDERATION_ACTIVE_4OF4_OK | tee -a "$LOG"

remote "$OBSERVER" "systemctl enable --now '$OBS_SERVICE'"
for n in $(seq 1 90);do
 if remote "$OBSERVER" "curl -fsS http://127.0.0.1:18019/health | python3 -c 'import json,sys;d=json.load(sys.stdin);assert d[\"ok\"] and d[\"role\"]==\"observer\" and d[\"chain_id\"]==4735489 and d[\"peer_count\"]>=1'" >/dev/null 2>&1;then break;fi
 sleep 2
done
remote "$OBSERVER" "curl -fsS http://127.0.0.1:18019/health | python3 -c 'import json,sys;d=json.load(sys.stdin);assert d[\"ok\"] and d[\"role\"]==\"observer\" and d[\"chain_id\"]==4735489 and d[\"peer_count\"]>=1'"
echo HVM_R8B_OBSERVER_BACKED_RPC_RUNTIME_OK | tee -a "$LOG"

copy "$REPO/hvm-mainnet-evm-gateway" "$OBSERVER" "$GW_OPT/hvm-mainnet-evm-gateway"
copy "$DEPLOY/mainnet-evm-routes.conf" "$OBSERVER" /etc/nginx/snippets/hvm-mainnet-evm.conf
copy "$DEPLOY/diagnose-mainnet-nginx.sh" "$OBSERVER" /root/hashburst-hvm-mainnet-nginx-diagnostics.sh
remote "$OBSERVER" "set -euo pipefail
 chown root:root '$GW_OPT/hvm-mainnet-evm-gateway' /etc/nginx/snippets/hvm-mainnet-evm.conf;chmod 0755 '$GW_OPT/hvm-mainnet-evm-gateway';chmod 0644 /etc/nginx/snippets/hvm-mainnet-evm.conf;chmod 0700 /root/hashburst-hvm-mainnet-nginx-diagnostics.sh
 cat > /etc/systemd/system/$GW_SERVICE <<'UNIT'
[Unit]
Description=HashBurst HVM mainnet public EVM gateway
After=network-online.target hashburst-hvm-mainnet-ingress.service
Requires=hashburst-hvm-mainnet-ingress.service
[Service]
DynamicUser=yes
ExecStart=/opt/hashburst-hvm-mainnet-evm-gateway/hvm-mainnet-evm-gateway
Restart=on-failure
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
MemoryMax=256M
TasksMax=64
[Install]
WantedBy=multi-user.target
UNIT
 V=/etc/nginx/sites-available/blockchainapi.one.conf
 E=/etc/nginx/sites-enabled/blockchainapi.one.conf
 test -f \"\$V\";test -L \"\$E\"
 test \"\$(readlink -f \"\$E\")\" = \"\$(readlink -f \"\$V\")\"
 cp -f \"\$V\" /root/.hvm-mainnet-vhost.pre-r9
 python3 - <<'PY'
from pathlib import Path
p=Path('/etc/nginx/sites-available/blockchainapi.one.conf')
lines=p.read_text().splitlines()
blocks=[]; depth=0; start=None
for i,line in enumerate(lines):
    stripped=line.split('#',1)[0]
    if start is None and stripped.strip().startswith('server') and '{' in stripped:
        start=i
    depth += stripped.count('{') - stripped.count('}')
    if start is not None and depth == 0:
        blocks.append((start,i)); start=None
targets=[]
for a,b in blocks:
    block='\n'.join(lines[a:b+1])
    if 'server_name' in block and 'blockchainapi.one' in block and any('listen' in x and '443' in x and 'ssl' in x for x in lines[a:b+1]):
        targets.append((a,b))
assert len(targets)==1, f'expected one blockchainapi.one HTTPS server, found {len(targets)}'
a,b=targets[0]
anchor='    include /etc/nginx/snippets/hvm-testnet-routes.conf;'
mainnet='    include /etc/nginx/snippets/hvm-mainnet-evm.conf;'
inside=lines[a:b+1]
if mainnet in lines:
    assert lines.count(mainnet)==1 and mainnet in inside, 'mainnet include exists outside canonical HTTPS server'
else:
    assert inside.count(anchor)==1, 'testnet route anchor missing or ambiguous in HTTPS server'
    pos=a+inside.index(anchor)+1
    lines.insert(pos,mainnet)
    p.write_text('\n'.join(lines)+'\n')
PY
 systemctl daemon-reload;systemctl enable --now '$GW_SERVICE'
 for n in \$(seq 1 30); do
   out=\$(curl -fsS --max-time 5 -H 'Content-Type: application/json' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"eth_chainId\",\"params\":[]}' http://127.0.0.1:18021/rpc 2>/dev/null || true)
   echo "\$out" | grep -q '"0x484201"' && break
   sleep 1
 done
 echo "\$out" | grep -q '"0x484201"'
 echo HVM_R8B_MAINNET_EVM_GATEWAY_LOCAL_OK
 nginx -t;systemctl reload nginx
 nginx -T 2>&1 | grep -Fq 'include /etc/nginx/snippets/hvm-mainnet-evm.conf;'
 local_tls=
 for n in \$(seq 1 15); do
   local_tls=\$(curl -sS --max-time 15 --resolve blockchainapi.one:443:127.0.0.1 -H 'Content-Type: application/json' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"eth_chainId\",\"params\":[]}' https://blockchainapi.one/api/hashburst/hvm/mainnet/evm 2>/dev/null || true)
   echo "\$local_tls" | grep -q '"0x484201"' && break
   sleep 1
 done
 if ! echo "\$local_tls" | grep -q '"0x484201"'; then
   /root/hashburst-hvm-mainnet-nginx-diagnostics.sh || true
   exit 22
 fi
 echo HVM_R8B_NGINX_LOCAL_TLS_SNI_ROUTE_OK
 systemctl is-active --quiet hashburst-hvm-testnet-ingress.service"
PUBLIC=https://blockchainapi.one/api/hashburst/hvm/mainnet/evm
for n in $(seq 1 30);do
 out=$(curl -fsS --max-time 15 -H 'Content-Type: application/json' --data '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}' "$PUBLIC" 2>/dev/null||true)
 grep -q '"0x484201"' <<<"$out"&&break;sleep 2
done
grep -q '"0x484201"' <<<"$out" || fail "public mainnet RPC unavailable"
blocked=$(curl -fsS --max-time 15 -H 'Content-Type: application/json' --data '{"jsonrpc":"2.0","id":2,"method":"admin_peers","params":[]}' "$PUBLIC")
grep -q -- '-32601' <<<"$blocked" || fail "administrative RPC not blocked"
echo HVM_R8B_OBSERVER_BACKED_PUBLIC_RPC_INGRESS_ACCEPTANCE_OK | tee -a "$LOG"

copy "$REPO/hvm-mainnet-production-canary" 77.90.188.153 /opt/hashburst-apow-mainnet/hvm-mainnet-production-canary
remote 77.90.188.153 "chmod 0755 /opt/hashburst-apow-mainnet/hvm-mainnet-production-canary; chown root:root /opt/hashburst-apow-mainnet/hvm-mainnet-production-canary"
for n in $(seq 1 90);do
 bal=$(curl -fsS -H 'Content-Type: application/json' --data '{"jsonrpc":"2.0","id":1,"method":"eth_getBalance","params":["0x0957c1f8802a58cf1E0eD1E866990Ea66f16236f","latest"]}' "$PUBLIC"|python3 -c 'import json,sys;print(int(json.load(sys.stdin)["result"],16))')
 ((bal>0))&&break;sleep 2
done
((bal>0))||fail "R8A miner identity did not receive finalized APoW reward"
remote 77.90.188.153 "/opt/hashburst-apow-mainnet/hvm-mainnet-production-canary --key /var/lib/hashburst-apow-mainnet/miner.key --out /tmp/mainnet-r8c-r8d-canary.json"
SSH_AUTH_SOCK="$SOCK" "${SCP[@]}" root@77.90.188.153:/tmp/mainnet-r8c-r8d-canary.json "$REPO/mainnet-r8c-r8d-canary.json"
echo HVM_R8C_R8D_PUBLIC_CANARY_EVIDENCE_COLLECTED_OK | tee -a "$LOG"

python3 - "$EVIDENCE" "$GITHUB_SHA" "$RUNTIME_SHA" "$PEER" "$R8A_RUN" "$R8A_ARTIFACT_DIGEST" "$REPO/mainnet-r8c-r8d-canary.json" <<'PY'
import hashlib,json,sys,time
out,source,runtime,peer,r8run,r8digest,canary=sys.argv[1:]
c=json.load(open(canary))
e={"schema":1,"source_commit":source,"chain_id":4735489,"runtime_sha256":runtime,"observer_peer_id":peer,"r8a_run_id":int(r8run),"r8a_artifact_digest":"sha256:"+r8digest,"canary":c,"gates":{"r8b_observer_rpc":True,"r8c_wallet_receipt_logs":True,"r8d_websocket":True,"r8e_manifest":True,"r9_activation":True},"founder_private_key_used":False,"validator_private_key_used_for_canary":False,"created_unix":int(time.time())}
raw=json.dumps(e,sort_keys=True,indent=2).encode()+b"\n";open(out,"wb").write(raw);print("manifest_sha256="+hashlib.sha256(raw).hexdigest())
PY
MANIFEST_SHA=$(sha256sum "$EVIDENCE"|awk '{print $1}')
echo "HVM_R8E_CANONICAL_PRODUCTION_RELEASE_MANIFEST_OK sha256=$MANIFEST_SHA" | tee -a "$LOG"

for ip in "${VALIDATORS[@]}";do remote "$ip" "systemctl is-active --quiet '$VSERVICE';systemctl is-enabled --quiet '$VSERVICE'";done
remote 77.90.188.153 "systemctl is-active --quiet '$APOW_SERVICE';systemctl is-enabled --quiet '$APOW_SERVICE'"
remote "$OBSERVER" "systemctl is-active --quiet '$OBS_SERVICE';systemctl is-enabled --quiet '$OBS_SERVICE';systemctl is-active --quiet '$GW_SERVICE';systemctl is-enabled --quiet '$GW_SERVICE';systemctl is-active --quiet hashburst-hvm-testnet-ingress.service"
rollback=0
trap - EXIT INT TERM
rm -rf "$TMP"
echo HVM_R9_FINAL_PRODUCTION_ACTIVATION_DEPLOY_ACCEPTANCE_OK | tee -a "$LOG"
echo MAINNET_CHAIN_ID_4735489_ACTIVE | tee -a "$LOG"
echo FOUNDER_PRIVATE_KEY_NOT_USED | tee -a "$LOG"
echo VALIDATOR_PRIVATE_KEYS_NOT_EXPORTED | tee -a "$LOG"
