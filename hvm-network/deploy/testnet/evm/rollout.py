#!/usr/bin/env python3
"""Five-node coordinated EVM migration. State/journals never rolled back."""
import argparse,base64,concurrent.futures,hashlib,importlib.util,json,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ssh_session',R/'ssh-session.py');transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)
NODES=[{'ip':'77.90.188.'+ip,'node_id':'hvm-testnet-v'+str(i),'role':'validator'} for i,ip in enumerate(('153','154','155','157'),1)]
# Observer node_id is read from its provisioned config and must be supplied in the plan.
def compare(rows,height,evm=True):
 fields=('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root','evm_state_root','evm_receipts_root','evm_gas_used')
 for r in rows:
  if r.get('chain_id')!=4735490 or r.get('height')!=height or not r.get('certificate'):raise RuntimeError('not a certified testnet commitment')
  if evm and (not r.get('evm_state_root') or not r.get('evm_receipts_root')):raise RuntimeError('EVM not active at common height')
 if any(tuple(r.get(k) for k in fields)!=tuple(rows[0].get(k) for k in fields) for r in rows):raise RuntimeError('common-height commitments differ')
 return rows[0]
def main():
 a=argparse.ArgumentParser();a.add_argument('action',choices=['check','activate','resume-migration','restart-v4']);a.add_argument('--plan',required=True);a.add_argument('--binary');args=a.parse_args()
 plan=json.loads(Path(args.plan).read_text());evm=plan['evm']
 if plan['chain_id']!=4735490 or evm['activation_height']<=0:raise RuntimeError('invalid testnet plan')
 nodes=NODES+[{'ip':'64.31.4.9','node_id':plan['observer_node_id'],'role':'observer'}]
 logs=Path(tempfile.mkdtemp(prefix='evm-live-',dir=R));sessions={}
 def call(n,action,**kw):
  result=sessions[n['ip']].call({'node':n,'action':action,'_timeout':3750 if action=='migrate' else 40,**kw})
  (logs/(str(time.time_ns())+'-'+n['ip']+'-'+action+'.json')).write_text(json.dumps(result,indent=2));return result
 def batch(action,**kw):
  with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:return list(pool.map(lambda n:call(n,action,**kw),nodes))
 def agreement(active=True):
  states=batch('status')
  if any(not s.get('reactor_running') or s.get('peer_count',0)<1 for s in states):raise RuntimeError('node not ready')
  if len({s.get('config_digest') for s in states})!=1 or not states[0].get('config_digest'):raise RuntimeError('consensus configuration digests differ')
  height=min(s['finalized_height'] for s in states)
  return compare(batch('proof',height=height),height,active)
 def wait_progress():
  start=None;until=time.monotonic()+7200;last=''
  while time.monotonic()<until:
   try:
    proof=agreement();height=proof['height']
    if height>=evm['activation_height']:
     if start is None:start=height
     if height>=start+5:return proof
    last='common_height='+str(height)
   except Exception as e:last=str(e)
   print('WAIT '+last,flush=True);time.sleep(10)
  raise RuntimeError('bounded readiness/finality timeout: '+last)
 try:
  for n in nodes:
   print('SSH_AUTH='+n['ip'],flush=True);sessions[n['ip']]=transport.Session(transport.ssh_command(n['ip']),(R/'node-rollout.py').read_text())
  if args.action in ('activate','resume-migration'):
   raw=Path(args.binary).read_bytes();sha=hashlib.sha256(raw).hexdigest()
   if sha!=plan['binary_sha256']:raise RuntimeError('release digest differs from plan')
   if args.action=='activate':
    agreement(False)
    states=batch('status')
    if max(s['finalized_height'] for s in states)+1000>=evm['activation_height']:raise RuntimeError('activation height too close; no service stopped')
   batch('stage',binary=base64.b64encode(raw).decode(),sha256=sha)
   batch('stop') # all stopped before any migration; prevents passing activation mid-rollout
   batch('migrate',sha256=sha,evm=evm)
   batch('start')
  proof=wait_progress();print('FIVE_NODE_EVM_COMMON_HEIGHT_OK='+str(proof['height']),flush=True)
  if args.action=='restart-v4':
   call(nodes[3],'restart');proof=wait_progress();call(nodes[3],'recovery');print('V4_EVM_RESTART_CATCHUP_OK='+str(proof['height']))
  batch('preservation');print('EVM_LIVE_GATE_OK action='+args.action)
 finally:
  for s in sessions.values():s.close()
  print('LOGS='+str(logs))
if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:print('STOP: retain state and journals; inspect evidence before resuming: '+str(e));raise SystemExit(1)
