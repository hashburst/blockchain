#!/usr/bin/env bash
# Read-only inventory of the known TESTNET fleet. No stop/restart/migration.
set -euo pipefail
umask 077
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
OUT=$(mktemp -d "$ROOT/fleet-inspection.XXXXXXXX")
echo "REPORT=$OUT"
for spec in '77.90.188.153 hvm-testnet-v1 validator' '77.90.188.154 hvm-testnet-v2 validator' '77.90.188.155 hvm-testnet-v3 validator' '77.90.188.157 hvm-testnet-v4 validator' '64.31.4.9 hvm-testnet-ingress observer'; do
  read -r host node role <<< "$spec"
  ssh -T -o ControlMaster=no -o ControlPath=none -o ConnectTimeout=10 \
    -o ServerAliveInterval=15 -o ServerAliveCountMax=4 "root@$host" \
    "python3 - '$node' '$role'" > "$OUT/$host.json.tmp" <<'PY'
import json,sys,subprocess,urllib.request
from pathlib import Path
node,role=sys.argv[1:]
suffix='-ingress' if role=='observer' else ''
config=Path('/etc/hashburst-hvm-testnet'+suffix+'/node.json')
unit='hashburst-hvm-testnet'+suffix+'.service'
c=json.loads(config.read_text())
assert c['node_id']==node and c['role']==role
assert c['network']=='testnet' and c['protocol']['chain_id']==4735490
state=subprocess.run(['systemctl','show',unit,'-p','ActiveState','-p','SubState','-p','MainPID','-p','NRestarts'],check=True,capture_output=True,text=True).stdout
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
with opener.open('http://'+c['rpc_listen']+'/health',timeout=20) as response:health=json.load(response)
assert health['chain_id']==4735490 and health['node_id']==node
print(json.dumps({'node':node,'role':role,'chain_id':4735490,'service':state,'health':health,
 'data_directory':c['data_dir'],'source_format':'must inspect actual payload; names alone do not identify codec',
 'no_service_changed':True},indent=2))
PY
  mv "$OUT/$host.json.tmp" "$OUT/$host.json"
  echo "READ_ONLY_NODE_CHECKED=$node"
done
echo 'FIVE_RESPONSES_COLLECTED_NOT_A_COMMON_HEIGHT_OR_MAINNET_CERTIFICATE'
