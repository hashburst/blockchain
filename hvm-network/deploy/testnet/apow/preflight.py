#!/usr/bin/env python3
"""Read-only five-node gate; no stop/start, migration, key reads or retries of writes."""
import argparse, concurrent.futures, importlib.util, json, tempfile, time, tarfile, hashlib
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ssh_session',R/'ssh-session.py')
transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)
TARGETS=[('77.90.188.153','hvm-testnet-v1'),('77.90.188.154','hvm-testnet-v2'),('77.90.188.155','hvm-testnet-v3'),('77.90.188.157','hvm-testnet-v4'),('64.31.4.9','hvm-testnet-ingress')]
FIELDS=('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root','evm_state_root','evm_receipts_root','evm_gas_used')
SOURCE=r'''
import hashlib,json,subprocess,urllib.request
from pathlib import Path
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
def get(path,data=None):
 req=urllib.request.Request('http://127.0.0.1:18009'+path,data,{'Content-Type':'application/json'})
 with opener.open(req,timeout=20) as r:return json.load(r)
def main(p):
 if p['action']=='commitment':
  d=get('/rpc',json.dumps({'jsonrpc':'2.0','id':1,'method':'hb_getFinalizedCommitment','params':[p['height']]}).encode())
  if d.get('id')!=1 or 'error' in d:raise RuntimeError(str(d))
  return d['result']
 observer=p['node_id']=='hvm-testnet-ingress'
 unit='hashburst-hvm-testnet'+('-ingress' if observer else '')
 c=json.loads((Path('/etc')/unit/'node.json').read_text());h=get('/health')
 if c['node_id']!=p['node_id'] or h['node_id']!=p['node_id'] or c['network']!='testnet' or h['chain_id']!=4735490 or c['protocol']['chain_id']!=4735490:raise RuntimeError('identity/chain mismatch')
 if h.get('role')!=c['role'] or c['role']!=('observer' if observer else 'validator') or (observer and c.get('consensus_key_file')):raise RuntimeError('role/signing mismatch')
 if not h.get('reactor_running') or h.get('peer_count',0)<1 or not c['protocol'].get('evm'):raise RuntimeError('runtime not ready')
 pin=(Path(c['data_dir'])/'runtime.pin').read_text().splitlines()
 if pin!=[h['config_digest'],c['node_id'],c['peer_id'],c.get('validator_id','')] or h['peer_id']!=c['peer_id']:raise RuntimeError('pin/health identity or digest mismatch')
 status=subprocess.run(['systemctl','show',unit+'.service','--property=ActiveState,SubState,MainPID,ExecMainStatus'],capture_output=True,text=True,timeout=10,check=True).stdout
 pid=int(dict(line.split('=',1) for line in status.splitlines() if '=' in line).get('MainPID','0'))
 if pid<=0:raise RuntimeError('no running runtime process')
 binary=hashlib.sha256()
 with open('/proc/'+str(pid)+'/exe','rb') as f:
  for chunk in iter(lambda:f.read(1048576),b''):binary.update(chunk)
 return {'binary_sha256':binary.hexdigest(),'node_id':c['node_id'],'role':c['role'],'height':h['finalized_height'],'digest':h['config_digest'],'protocol':c['protocol'],'peer_id':h['peer_id'],'peer_count':h['peer_count'],'genesis':c['genesis_hash'],'pin_sha256':hashlib.sha256((Path(c['data_dir'])/'runtime.pin').read_bytes()).hexdigest(),'service':status}
'''
def validate_rows(rows):
 if len(rows)!=5 or {r['node_id'] for r in rows}!={n for _,n in TARGETS}:raise ValueError('five known nodes required')
 if len({r['digest'] for r in rows})!=1 or len({json.dumps(r['protocol'],sort_keys=True) for r in rows})!=1 or len({r['genesis'] for r in rows})!=1:raise ValueError('network configurations differ')
 if len({r['peer_id'] for r in rows})!=5:raise ValueError('duplicate P2P identity')
 for r in rows:
  if 'ActiveState=active' not in r['service'] or 'SubState=running' not in r['service']:raise ValueError('service not running: '+r['node_id'])
def compare(proofs,height):
 if len(proofs)!=5:raise ValueError('five proofs required')
 for p in proofs:
  if p.get('chain_id')!=4735490 or p.get('height')!=height or not p.get('certificate'):raise ValueError('wrong height/chain or missing QC')
  if any(p.get(k) is None for k in FIELDS) or any(not p.get(k) for k in FIELDS if k not in ('height','evm_gas_used','chain_id')):raise ValueError('missing finalized roots')
 if any(tuple(p[k] for k in FIELDS)!=tuple(proofs[0][k] for k in FIELDS) for p in proofs[1:]):raise ValueError('finalized commitments differ')
def main():
 a=argparse.ArgumentParser();a.add_argument('--out-dir');args=a.parse_args()
 out=Path(tempfile.mkdtemp(prefix='apow-preflight-',dir=args.out_dir or R));sessions=[]
 expected=hashlib.sha256((R/'hashburst-testnet').read_bytes()).hexdigest() if (R/'hashburst-testnet').exists() else None
 try:
  for host,name in TARGETS:
   print('SSH_AUTH='+host,flush=True);sessions.append(transport.Session(transport.ssh_command(host),SOURCE))
  with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
   rows=list(pool.map(lambda item:item[0].call({'action':'status','node_id':item[1],'_timeout':40}),zip(sessions,[n for _,n in TARGETS])))
   (out/'nodes.json').write_text(json.dumps(rows,indent=2)+'\n');validate_rows(rows)
   height=min(r['height'] for r in rows)
   proofs=list(pool.map(lambda s:s.call({'action':'commitment','height':height,'_timeout':40}),sessions))
   (out/'commitments.json').write_text(json.dumps(proofs,indent=2)+'\n');compare(proofs,height)
   time.sleep(5)
   later=list(pool.map(lambda item:item[0].call({'action':'status','node_id':item[1],'_timeout':40}),zip(sessions,[n for _,n in TARGETS])))
   (out/'progress.json').write_text(json.dumps(later,indent=2)+'\n');validate_rows(later)
   if any(b['height']<=a['height'] or b['digest']!=a['digest'] or b['pin_sha256']!=a['pin_sha256'] for a,b in zip(rows,later)):raise RuntimeError('finality not progressing or configuration changed; evidence retained')
  report={'ok':True,'chain_id':4735490,'common_height':height,'block_hash':proofs[0]['hash'],'config_digest':rows[0]['digest'],'apow_already_configured':bool(rows[0]['protocol'].get('apow')),'release_binary_sha256':expected,'already_updated_nodes':[r['node_id'] for r in rows if expected and r['binary_sha256']==expected],'no_service_changed':True,'no_private_key_read':True,'activation_authorized_by_this_report':False,'qc_signatures_independently_verified':False}
  (out/'READY.json').write_text(json.dumps(report,indent=2)+'\n');print('FIVE_NODE_FINALIZED_AGREEMENT_OK height='+str(height));print('READ_ONLY_PREFLIGHT_OK_NO_SERVICE_CHANGED')
 except Exception as e:
  (out/'ERROR.txt').write_text(str(e)+'\n');raise
 finally:
  for s in sessions:s.close()
  print('REPORT='+str(out),flush=True)
  archive=out.with_suffix('.tar.gz')
  with tarfile.open(archive,'w:gz') as tar:tar.add(out,arcname=out.name)
  print('REPORT_ARCHIVE='+str(archive),flush=True)
if __name__=='__main__':main()
