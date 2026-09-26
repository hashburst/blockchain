#!/usr/bin/env python3
"""Read-only plan creation. This does not stop, migrate or start a service."""
import argparse,hashlib,importlib.util,json
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('transport',R/'ssh-session.py');transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)
SOURCE=r'''
import json,urllib.request
from pathlib import Path
def main(p):
 name='hashburst-hvm-testnet'+('-ingress' if p['observer'] else '')
 c=json.loads((Path('/etc')/name/'node.json').read_text())
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open('http://127.0.0.1:18009/health',timeout=20) as r:h=json.load(r)
 if c['protocol']['chain_id']!=4735490 or h['chain_id']!=4735490 or c['node_id']!=h['node_id'] or c['protocol'].get('evm'):raise RuntimeError('expected unmigrated testnet')
 if not h.get('reactor_running') or h.get('peer_count',0)<1:raise RuntimeError('node not ready')
 if p['observer'] and (c['role']!='observer' or c.get('consensus_key_file')):raise RuntimeError('unsigned observer required')
 return {'node_id':c['node_id'],'role':c['role'],'height':h['finalized_height'],'digest':h['config_digest']}
'''
def main():
 p=argparse.ArgumentParser();p.add_argument('--binary',required=True);p.add_argument('--out',required=True);p.add_argument('--margin',type=int,default=10000);p.add_argument('--gas-limit',type=int,default=1000000);p.add_argument('--base-fee-wei',type=int,default=1);a=p.parse_args()
 if a.margin<=1000 or not 21000<=a.gas_limit<=30000000 or not 1<=a.base_fee_wei<2**64:raise RuntimeError('invalid activation parameters')
 rows=[]
 for i,ip in enumerate(('77.90.188.153','77.90.188.154','77.90.188.155','77.90.188.157','64.31.4.9')):
  s=transport.Session(transport.ssh_command(ip),SOURCE)
  try:r=s.call({'observer':i==4})
  finally:s.close()
  if i<4 and (r['node_id']!='hvm-testnet-v'+str(i+1) or r['role']!='validator'):raise RuntimeError('validator identity mismatch')
  rows.append(r)
 if len({r['digest'] for r in rows})!=1:raise RuntimeError('network configuration mismatch')
 plan={'chain_id':4735490,'binary_sha256':hashlib.sha256(Path(a.binary).read_bytes()).hexdigest(),'observer_node_id':rows[4]['node_id'],'evm':{'activation_height':max(r['height'] for r in rows)+a.margin,'gas_limit':a.gas_limit,'base_fee_wei':a.base_fee_wei},'observed':rows}
 with open(a.out,'x') as f:json.dump(plan,f,indent=2)
 print(json.dumps(plan,indent=2));print('PLAN_WRITTEN_NO_SERVICE_CHANGED')
if __name__=='__main__':main()
