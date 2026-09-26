#!/usr/bin/env bash
# Run on the Mac or coordinator. Remote reads only; no services or keys created.
set -u
package="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
command -v python3 >/dev/null || exit 1
command -v ssh >/dev/null || exit 1
out="$(mktemp -d "$PWD/hvm-testnet-preflight-XXXXXX")" || exit 1
for ip in 77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157; do
  echo "READ_ONLY_PREFLIGHT=$ip"
  if ssh -T -o ConnectTimeout=8 -o StrictHostKeyChecking=ask "root@$ip" \
    "python3 - '$ip'" < "$package/preflight-node.py" > "$out/$ip.json"; then
    echo "PREFLIGHT_OK=$ip"
  else
    echo "PREFLIGHT_FAILED=$ip (report retained; no changes applied)"
  fi
done
python3 - "$out" <<'PY'
import json
from pathlib import Path
import sys
root=Path(sys.argv[1]);reports=[]
for ip in ('77.90.188.153','77.90.188.154','77.90.188.155','77.90.188.157'):
    try:
        d=json.loads((root/(ip+'.json')).read_text())
    except Exception:
        d={'expected_ip':ip,'ok':False,'errors':['SSH failed or invalid output']}
    reports.append(d)
keys=[d.get('tep_public_key') for d in reports]
ids=[d.get('tep_node_id') for d in reports]
ok=all(d.get('ok') is True for d in reports) and len(set(keys))==4 and len(set(ids))==4
summary={'schema':1,'chain_id':4735490,'ok':ok,'nodes':reports,
         'unique_tep_keys':len(set(keys))==4 and None not in keys,
         'unique_tep_node_ids':len(set(ids))==4 and None not in ids}
file=root/'preflight-summary.json';file.write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps(summary,indent=2));print('REPORT='+str(file))
print('NO_SERVICE_STARTED_NO_PRIVATE_KEY_READ')
raise SystemExit(0 if ok else 1)
PY
