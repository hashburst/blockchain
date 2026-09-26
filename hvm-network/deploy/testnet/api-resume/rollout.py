import base64,concurrent.futures,hashlib,importlib.util,json,os,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parent
def load(name,file):
 s=importlib.util.spec_from_file_location(name,ROOT/file);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
transport=load('transport','ssh-session.py');compare=load('compare','compare-finalized.py')
def main():
 os.umask(0o077)
 for line in (ROOT/'SHA256SUMS').read_text().splitlines():
  digest,name=line.split('  ',1)
  if hashlib.sha256((ROOT/name).read_bytes()).hexdigest()!=digest:raise RuntimeError('checksum '+name)
 nodes=json.loads((ROOT/'targets.json').read_text());sessions={}
 logs=Path(tempfile.mkdtemp(prefix='api-rollout-',dir=ROOT))
 def call(n,action,**kw):
  result=sessions[n['node_id']].call(dict(node=n,action=action,_timeout=3720 if action=='upgrade' else 30,**kw))
  (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'.json')).write_text(json.dumps(result,indent=2))
  return result
 def batch(action,**kw):
  with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
   return list(pool.map(lambda n:call(n,action,**kw),nodes))
 try:
  for n in nodes:
   print('SSH_AUTH='+n['ip'],flush=True)
   sessions[n['node_id']]=transport.Session(transport.ssh_command(n['ip']),(ROOT/'node-api.py').read_text())
  raw=(ROOT/'hashburst-testnet').read_bytes()
  for n in nodes:
   batch('health')
   print('CHECK_OR_UPGRADE='+n['node_id']+' (bounded replay wait: 60 minutes; do not restart manually)',flush=True)
   result=call(n,'upgrade',binary=base64.b64encode(raw).decode(),sha256=hashlib.sha256(raw).hexdigest())
   print(('ALREADY_UPDATED_NO_RESTART=' if result.get('no_restart') else 'UPGRADE_RECOVERY_OK=')+n['node_id'],flush=True)
  rows=batch('health');height=min(r['finalized_height'] for r in rows)
  proofs=batch('proof',height=height)
  compare.compare(proofs,height)
  print('HVM_TESTNET_FIXED_HEIGHT_AGREEMENT_OK height='+str(height),flush=True)
 finally:
  for s in sessions.values():s.close()
  print('LOGS='+str(logs),flush=True)
if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:
  print('STOP: state and journals retained; do not rerun blindly. '+str(e));raise SystemExit(1)
