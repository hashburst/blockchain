#!/usr/bin/env python3
"""Five-node coordinated pacemaker update; preserve the existing EVM migration."""
import argparse,base64,concurrent.futures,hashlib,importlib.util,json,sys,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ssh_session',R/'ssh-session.py');ssh=importlib.util.module_from_spec(spec);spec.loader.exec_module(ssh)
NODES=[{'ip':ip,'node_id':'hvm-testnet-v'+str(i+1),'role':'validator'} for i,ip in enumerate(('77.90.188.153','77.90.188.154','77.90.188.155','77.90.188.157'))]+[{'ip':'64.31.4.9','node_id':'hvm-testnet-ingress','role':'observer'}]
FIELDS=('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root','evm_state_root','evm_receipts_root','evm_gas_used')
def compare(rows,height):
 for r in rows:
  if r.get('chain_id')!=4735490 or r.get('height')!=height or not r.get('certificate'):raise RuntimeError('invalid finalized commitment')
  if height>=53303 and (not r.get('evm_state_root') or not r.get('evm_receipts_root')):raise RuntimeError('missing EVM commitments')
 if any(tuple(r.get(k) for k in FIELDS)!=tuple(rows[0].get(k) for k in FIELDS) for r in rows[1:]):raise RuntimeError('same-height commitments disagree')
class Progress:
 def __init__(self):self.height=None;self.changed=None
 def update(self,height,now):
  if self.height is None or height>self.height:self.height=height;self.changed=now
  if height<self.height:raise RuntimeError('finalized height regressed')
  if now-self.changed>=600:raise RuntimeError('FINALITY_STALLED_600_SECONDS; services retained; inspect per-node states')
def main():
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=('apply','verify','start-prepared','restart-v4'));a=parser.parse_args()
 for line in (R/'SHA256SUMS').read_text().splitlines():
  sha,name=line.split('  ',1)
  if hashlib.sha256((R/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('checksum '+name)
 raw=(R/'hashburst-testnet').read_bytes();sha=hashlib.sha256(raw).hexdigest();source=(R/'node.py').read_text()
 logs=Path(tempfile.mkdtemp(prefix='pacemaker-results-',dir=R));sessions={}
 def call(n,action,**kw):
  try:
   r=sessions[n['ip']].call({'node':n,'action':action,'sha256':sha,'_timeout':180,**kw})
   (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'.json')).write_text(json.dumps(r,indent=2));return r
  except Exception as e:
   (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'-error.txt')).write_text(str(e));raise
 def batch(action,**kw):
  with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
   jobs=[pool.submit(call,n,action,**kw) for n in NODES];rows=[];errors=[]
   for n,f in zip(NODES,jobs):
    try:rows.append(f.result())
    except Exception as e:errors.append(n['node_id']+': '+str(e))
   if errors:raise RuntimeError('; '.join(errors))
   return rows
 try:
  for n in NODES:
   print('SSH_AUTH='+n['ip'],flush=True);sessions[n['ip']]=ssh.Session(ssh.ssh_command(n['ip']),source)
  if a.action=='apply':
   for step,kw in [('stage',{'binary':base64.b64encode(raw).decode()}),('stop',{}),('install',{}),('start',{})]:
    print('FIVE_NODE_'+step.upper(),flush=True);batch(step,**kw)
  elif a.action=='start-prepared':batch('prepared');batch('start')
  elif a.action=='restart-v4':
   gate=json.loads((R/'GATE-pacemaker.json').read_text())
   if gate.get('binary_sha256')!=sha or gate.get('height',0)<53303 or time.time()-gate['timestamp']>86400:raise RuntimeError('recent finality gate required')
   batch('release');call(NODES[3],'restart')
  end=time.monotonic()+7200;progress=Progress();activated=None;ready=False;report=0
  while time.monotonic()<end:
   try:rows=batch('status')
   except Exception as e:
    if any(s.broken for s in sessions.values()):raise RuntimeError('SSH lost; reconnect using verify; no mutation retried') from e
    if time.monotonic()-report>=60:
     for n,d in zip(NODES,batch('diagnostic')):
      print('RUNTIME '+n['node_id']+' '+json.dumps(d),flush=True)
      if d['systemd']['ActiveState']=='failed' or d['systemd']['SubState']=='auto-restart':raise RuntimeError('runtime failed; state retained')
     report=time.monotonic()
    print('REPLAY_WAIT '+str(e),flush=True);time.sleep(15);continue
   summary=[{k:r.get(k) for k in ('node_id','finalized_height','peer_count','reactor')} for r in rows]
   print('STATE '+json.dumps(summary),flush=True)
   if not all(r.get('reactor_running') and r.get('peer_count',0)>=1 for r in rows):
    if ready:raise RuntimeError('reactor/peers lost after readiness')
    time.sleep(15);continue
   if len({r['config_digest'] for r in rows})!=1:raise RuntimeError('config digests differ')
   ready=True;height=min(r['finalized_height'] for r in rows);compare(batch('proof',height=height),height)
   progress.update(height,time.monotonic())
   print('COMMON_FINALITY='+str(height),flush=True)
   if height>=53303:
    if activated is None:activated=height
    elif height>=activated+5:
     batch('release')
     if a.action=='restart-v4':call(NODES[3],'restart-check');print('V4_PERSISTENT_RESTART_OK',flush=True)
     (R/'GATE-pacemaker.json').write_text(json.dumps({'ok':True,'chain_id':4735490,'binary_sha256':sha,'height':height,'logs':str(logs),'timestamp':time.time()},indent=2))
     print('HVM_TESTNET_PACEMAKER_AND_EVM_FINALITY_OK',flush=True);return
   time.sleep(15)
  raise RuntimeError('bounded replay/progress wait expired; use verify; do not reapply')
 finally:
  for s in sessions.values():s.close()
  print('LOGS='+str(logs),flush=True)
if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:print('STOP: data, journals and services retained: '+str(e));sys.exit(1)
