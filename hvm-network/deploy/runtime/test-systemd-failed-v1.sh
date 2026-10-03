#!/usr/bin/env bash
# Disposable CI host only. Fake health server tests orchestration, not consensus.
set -euo pipefail
[ "${HVM_DISPOSABLE_CI:-}" = 1 ] || { echo 'Disposable CI opt-in required'; exit 1; }
[ "$(id -u)" = 0 ]
[ "$(ps -p 1 -o comm= | tr -d ' ')" = systemd ]
[ ! -e /etc/hashburst-hvm-testnet ]
[ ! -e /etc/systemd/system/hashburst-hvm-testnet.service ]
BASE=$(mktemp -d /var/tmp/hvm-installer-ci-XXXXXX)
export BASE
cleanup() {
 systemctl stop hashburst-hvm-testnet.service || true
}
trap cleanup EXIT
cat > "$BASE/fake.go" <<'GO'
package main
import("encoding/json";"net/http";"time";"os")
var release string
func main(){if release=="old"{os.Exit(1)};http.HandleFunc("/health",func(w http.ResponseWriter,r *http.Request){json.NewEncoder(w).Encode(map[string]interface{}{"chain_id":4735490,"node_id":"hvm-testnet-v1","peer_id":"ci-peer","role":"validator","reactor_running":true,"peer_count":3,"finalized_height":time.Now().Unix(),"fixture":release})});panic(http.ListenAndServe("127.0.0.1:18009",nil))}
GO
go build -ldflags='-X main.release=old' -o "$BASE/old" "$BASE/fake.go"
go build -ldflags='-X main.release=new' -o "$BASE/new" "$BASE/fake.go"
mkdir -p /etc/hashburst-hvm-testnet "$BASE/data"
python3 - <<'PY'
import os,json,hashlib
from pathlib import Path
b=Path(os.environ['BASE'])
Path('/etc/hashburst-hvm-testnet/node.json').write_text(json.dumps({'network':'testnet','protocol':{'chain_id':4735490},'node_id':'hvm-testnet-v1','role':'validator','peer_id':'ci-peer','data_dir':str(b/'data')}))
for name in ['runtime.pin','consensus-votes.jsonl','consensus-bft-signatures.jsonl']:(b/'data'/name).write_text('unchanged-fixture\n')
h=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
(b/'release.json').write_text(json.dumps({'chain_id':4735490,'source_commit':'0'*40,'sha256':h(b/'new'),'predecessors':[h(b/'old')], 'failed_node_recovery':{'node_id':'hvm-testnet-v1','predecessor_sha256':h(b/'old'),'result':'exit-code','exit_status':1}}))
PY
cat > /etc/systemd/system/hashburst-hvm-testnet.service <<EOF2
[Unit]
Description=Disposable HVM installer test fixture
[Service]
User=root
ExecStart=$BASE/old --config /etc/hashburst-hvm-testnet/node.json
EOF2
systemctl daemon-reload
systemctl start hashburst-hvm-testnet.service || true
for i in $(seq 1 20); do
 [ "$(systemctl show hashburst-hvm-testnet.service -p ActiveState --value)" = failed ] && break
 sleep 1
done
[ "$(systemctl show hashburst-hvm-testnet.service -p ActiveState --value)" = failed ]
python3 deploy/runtime/hashburst-install.py install --node hvm-testnet-v1 --release "$BASE/release.json" --binary "$BASE/new" --timeout 60
python3 - <<'PY'
import os,json,time
from pathlib import Path
b=Path(os.environ['BASE']);sha=json.loads((b/'release.json').read_text())['sha256'];p=Path('/var/lib/hashburst-runtime-installer/hvm-testnet-v1')/sha/'state.json'
for i in range(60):
 if p.exists() and json.loads(p.read_text())['phase']=='verified':break
 time.sleep(1)
else:raise SystemExit('worker failed to reach verified')
PY
BEFORE=$(systemctl show hashburst-hvm-testnet.service --property=MainPID --value)
python3 deploy/runtime/hashburst-install.py resume --node hvm-testnet-v1 --release "$BASE/release.json" --binary "$BASE/new" --timeout 60
python3 deploy/runtime/hashburst-install.py status --node hvm-testnet-v1 --release "$BASE/release.json" --binary "$BASE/new"
# The resumed worker may still own its lock; poll the existing process, no mutation.
sleep 10
AFTER=$(systemctl show hashburst-hvm-testnet.service --property=MainPID --value)
[ "$BEFORE" = "$AFTER" ]
[ "$AFTER" -gt 0 ]
echo SYSTEMD_FAILED_V1_RECOVERY_AND_RESUME_NO_RESTART_OK
# Upgrade a live runtime with the previous installer's override already present.
go build -ldflags='-X main.release=aligned' -o "$BASE/aligned" "$BASE/fake.go"
python3 - <<'PY'
import os,json,hashlib
from pathlib import Path
b=Path(os.environ['BASE']);h=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
(b/'aligned.json').write_text(json.dumps({'chain_id':4735490,'source_commit':'1'*40,'sha256':h(b/'aligned'),'predecessors':[h(b/'new')]}))
PY
python3 deploy/runtime/fleet-install.py install --node hvm-testnet-v1 --release "$BASE/aligned.json" --binary "$BASE/aligned" --timeout 60
python3 - <<'PY'
import os,json,time
from pathlib import Path
b=Path(os.environ['BASE']);sha=json.loads((b/'aligned.json').read_text())['sha256'];p=Path('/var/lib/hashburst-runtime-installer/hvm-testnet-v1')/sha/'state.json'
for i in range(60):
 if p.exists() and json.loads(p.read_text())['phase']=='verified':break
 time.sleep(1)
else:raise SystemExit('live upgrade did not verify')
PY
BEFORE=$(systemctl show hashburst-hvm-testnet.service --property=MainPID --value)
python3 deploy/runtime/fleet-install.py verify --node hvm-testnet-v1 --release "$BASE/aligned.json" --binary "$BASE/aligned" --timeout 60
AFTER=$(systemctl show hashburst-hvm-testnet.service --property=MainPID --value)
[ "$BEFORE" = "$AFTER" ]
echo SYSTEMD_LIVE_OVERRIDE_REPLACEMENT_AND_VERIFY_NO_RESTART_OK
