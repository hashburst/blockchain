#!/usr/bin/env python3
"""Resume the recorded partial activation; retain original rollout evidence."""
import argparse,base64,concurrent.futures,hashlib,importlib.util,json,subprocess,tempfile,time,sys
from pathlib import Path
R=Path(__file__).resolve().parent
ORIGINAL='69d9b71dacf2fbf51843789ac3c073a946b174022e7fcbc5cbd80a6ed11db298'
def load(name,file):
 path=R/file if (R/file).exists() else R.parent/file
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
transport=load('repair_ssh','ssh-session.py');common=load('repair_common','rollout.py')
def require_live_transports(sessions):
 broken=[ip for ip,s in sessions.items() if s.broken]
 if broken:raise RuntimeError('SSH_TRANSPORT_LOST: '+', '.join(broken)+'; services retained. Reconnect with verify; do not repeat migration.')
def validate_plan(p):
 if p.get('chain_id')!=4735490 or p.get('binary_sha256')!=ORIGINAL or p.get('observer_node_id')!='hvm-testnet-ingress':raise RuntimeError('not the original partial-rollout plan')
 if p.get('evm')!={'activation_height':53303,'gas_limit':200000,'base_fee_wei':1}:raise RuntimeError('original EVM parameters changed')
def main():
 a=argparse.ArgumentParser();a.add_argument('action',choices=['recover','start-prepared','verify','restart-v4']);a.add_argument('--plan',required=True);a.add_argument('--retry-offline',action='store_true');args=a.parse_args()
 plan=json.loads(Path(args.plan).read_text());validate_plan(plan)
 for line in (R/'SHA256SUMS').read_text().splitlines():
  sha,name=line.split('  ',1)
  if hashlib.sha256((R/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('package checksum '+name)
 raw=(R/'hashburst-testnet').read_bytes();sha=hashlib.sha256(raw).hexdigest();evm=plan['evm']
 nodes=common.NODES+[{'ip':'64.31.4.9','node_id':'hvm-testnet-ingress','role':'observer'}]
 logs=Path(tempfile.mkdtemp(prefix='evm-recovery-',dir=R));sessions={}
 source=(R/'node-rollout.py').read_text().replace('def main(p):','def base_main(p):',1)+'\n'+(R/'remote.py').read_text()
 def call(n,action,**kw):
  payload={'node':n,'action':action,'sha256':sha,'evm':evm,'_timeout':180 if action=='prepare' else 60,**kw}
  try:r=sessions[n['ip']].call(payload)
  except Exception as e:
   (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'-error.txt')).write_text(str(e));raise
  (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'.json')).write_text(json.dumps(r,indent=2));return r
 def batch(action,**kw):
  with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
   futures=[pool.submit(call,n,action,**kw) for n in nodes]
   results=[];errors=[]
   for n,f in zip(nodes,futures):
    try:results.append(f.result())
    except Exception as e:errors.append(n['node_id']+': '+str(e))
   if errors:raise RuntimeError('; '.join(errors))
   return results
 def progress():
  deadline=time.monotonic()+14400;start=None
  while time.monotonic()<deadline:
   try:
    states=batch('status')
    if any(not s.get('reactor_running') or s.get('peer_count',0)<1 for s in states):raise RuntimeError('reactor/peers not ready')
    if len({s.get('config_digest') for s in states})!=1 or not states[0].get('config_digest'):raise RuntimeError('configuration digests differ')
    height=min(s['finalized_height'] for s in states)
    common.compare(batch('proof',height=height),height,height>=evm['activation_height'])
    print('COMMON_FINALITY='+str(height)+' ACTIVATION='+str(evm['activation_height']),flush=True)
    if height>=evm['activation_height']:
     if start is None:start=height
     elif height>=start+5:return height
   except Exception as e:
    require_live_transports(sessions)
    print('WAIT '+str(e),flush=True)
   time.sleep(15)
  raise RuntimeError('finality wait expired; services retained; run verify after inspecting logs')
 try:
  for n in nodes:
   print('SSH_AUTH='+n['ip'],flush=True);sessions[n['ip']]=transport.Session(transport.ssh_command(n['ip']),source)
  if args.action=='recover':
   if any(r['service_state'] not in ('inactive','failed') for r in batch('inspect')):raise RuntimeError('all five services must be stopped; active services retained. Use verify if already started.')
   batch('prepare',files={'hashburst-testnet':base64.b64encode(raw).decode(),'offline.py':base64.b64encode((R/'offline.py').read_bytes()).decode()},retry_offline=args.retry_offline)
   deadline=time.monotonic()+14400
   while True:
    rows=batch('poll');done=True
    for n,r in zip(nodes,rows):
     if r.get('complete'):
      result=r['result']
      if not result.get('ok') or result.get('binary_sha256')!=sha or result.get('evm')!=evm:raise RuntimeError('offline result mismatch')
      print('OFFLINE_OK='+n['node_id'],flush=True)
     else:
      done=False;print('OFFLINE_WAIT='+n['node_id']+' '+json.dumps(r),flush=True)
      if r.get('state') not in ('active','activating'):raise RuntimeError('offline job failed; inspect the printed log; no services started')
    if done:break
    if time.monotonic()>deadline:raise RuntimeError('polling deadline; offline systemd jobs retained. Run recover again to reconnect, not activate.')
    time.sleep(30)
   print('FIVE_OFFLINE_GATES_OK',flush=True);batch('start')
  if args.action=='start-prepared':
   rows=batch('poll')
   if not all(r.get('complete') and r['result'].get('ok') and r['result'].get('binary_sha256')==sha and r['result'].get('evm')==evm for r in rows):raise RuntimeError('all five offline results required before starting any service')
   batch('start')
  height=progress();batch('release');batch('preservation')
  print('FIVE_NODE_EVM_COMMON_HEIGHT_OK='+str(height),flush=True)
  if args.action=='restart-v4':
   marker=call(nodes[3],'restart-status')
   if not marker['requested']:call(nodes[3],'restart');print('SINGLE_RESTART_REQUESTED=v4',flush=True)
   else:print('EXISTING_RESTART_PROOF_NO_SECOND_RESTART',flush=True)
   height=progress();batch('release');call(nodes[3],'recovery');batch('preservation')
   print('V4_EVM_RESTART_CATCHUP_OK='+str(height),flush=True)
  proof={'ok':True,'action':args.action,'chain_id':4735490,'binary_sha256':sha,'evm':evm,'height':height,'logs':str(logs),'timestamp':time.time()}
  (R/('GATE-'+args.action+'.json')).write_text(json.dumps(proof,indent=2));print('EVM_RECOVERY_GATE_OK action='+args.action,flush=True)
 finally:
  for s in sessions.values():s.close()
  print('LOGS='+str(logs),flush=True)
if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:print('STOP: state, journals and jobs retained: '+str(e),flush=True);sys.exit(1)
