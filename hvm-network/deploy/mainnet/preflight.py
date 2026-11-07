#!/usr/bin/env python3
"""Coordinator for read-only HVM mainnet preflight. No service mutation."""
import json,subprocess,tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent
NODES=[('77.90.188.153','validator'),('77.90.188.154','validator'),('77.90.188.155','validator'),('77.90.188.157','validator'),('64.31.4.9','observer')]
def main():
 out=Path(tempfile.mkdtemp(prefix='hvm-mainnet-preflight-'))
 rows=[]
 for ip,role in NODES:
  p=subprocess.run(['ssh','-T','-o','ControlMaster=no','-o','ControlPath=none','-o','ControlPersist=no','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=10','root@'+ip,'python3 - '+ip+' '+role],input=(ROOT/'preflight-node.py').read_text(),text=True,capture_output=True,timeout=30)
  try:r=json.loads(p.stdout)
  except Exception:r={'ok':False,'expected_ip':ip,'role':role,'errors':['SSH failed or invalid JSON']}
  (out/(ip+'.json')).write_text(json.dumps(r,indent=2)+'\n');rows.append(r)
 ok=len(rows)==5 and all(x.get('ok') is True for x in rows)
 summary={'schema':1,'chain_id':4735489,'mode':'read-only','ok':ok,'nodes':rows}
 (out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
 print(json.dumps(summary,indent=2));print('REPORT='+str(out/'summary.json'));print('MAINNET_PREFLIGHT_NO_MUTATION')
 raise SystemExit(0 if ok else 1)
if __name__=='__main__':main()
