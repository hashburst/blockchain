#!/usr/bin/env python3
"""Explicit plan, migration, offline barrier, start and recovery commands."""
import argparse,concurrent.futures,hashlib,importlib.util,json,tarfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
def module(name):
 spec=importlib.util.spec_from_file_location(name,R/(name+'.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
pre=module('preflight');measure=module('measure-all')
SOURCE=pre.SOURCE+'\ninspect=main\n'+(R/'stage-node.py').read_text()+'\nstage_dispatch=main\nUPLOAD_NAMES.add("hvm-apow-audit")\n'+(R/'migrate-node.py').read_text()
worker_source=SOURCE+"\nif __name__=='__main__':\n import sys\n if len(sys.argv)!=3 or sys.argv[1]!='--worker':raise SystemExit('worker arguments required')\n worker(sys.argv[2])\n"
ns={'__name__':'coordinator_rules'};exec(SOURCE,ns)
def measurement(path):
 with tarfile.open(path,'r:gz') as t:
  entries={}
  for m in t.getmembers():
   if not m.isfile() or m.size>4_000_000:continue
   name=Path(m.name).name
   if name in entries:raise ValueError('duplicate measurement member')
   if name.endswith('.json'):entries[name]=json.load(t.extractfile(m))
 proposal=entries['PARAMETERS_PROPOSAL.json']
 expected={'initial_bits':proposal['proposed_initial_bits'],'min_bits':proposal['proposed_min_bits'],'max_bits':proposal['proposed_max_bits'],'window':proposal['proposed_window'],'target_seconds':proposal['proposed_target_seconds']}
 if expected!=ns['PROFILE'] or proposal['activation_height']is not None or proposal['gas_limit_change']is not None:raise ValueError('measurement does not support this profile')
 proofs=entries['final-commitments.json'];pre.compare(proofs,proofs[0]['height'])
 return entries['progress.json']
def main():
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=['plan','migrate','jobs','start','verify','restart','restart-check','audit','miners','closeout']);parser.add_argument('--plan',default='activation-plan.json');parser.add_argument('--measurement');parser.add_argument('--timeout',type=int,default=14400);args=parser.parse_args()
 planpath=Path(args.plan).resolve();sessions=[]
 try:
  for host,node in pre.TARGETS:
   print('SSH_AUTH='+host,flush=True);sessions.append(pre.transport.Session(pre.transport.ssh_command(host),SOURCE))
  plan=None if args.action=='plan' else json.loads(planpath.read_text())
  if plan:ns['validate_plan'](plan)
  def batch(action,**extra):
   with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
    fs=[pool.submit(s.call,dict(action=action,node_id=node,plan=plan,_timeout=50,**extra)) for s,(_,node) in zip(sessions,pre.TARGETS)]
    results=[];errors=[]
    for (_,node),f in zip(pre.TARGETS,fs):
     try:results.append(f.result())
     except Exception as e:errors.append(node+': '+str(e))
    if errors:raise RuntimeError('; '.join(errors))
    return results
  def persist(name,data):
   out=planpath.parent/(planpath.stem+'-evidence');out.mkdir(mode=0o700,exist_ok=True)
   target=out/(name+'-'+str(time.time_ns())+'.json');target.write_text(json.dumps(data,indent=2)+'\n');target.chmod(0o600)
  if args.action=='plan':
   if planpath.exists() or not args.measurement:raise ValueError('new plan path and --measurement required')
   baseline={r['node_id']:r for r in measurement(args.measurement)}
   rows=batch('status');pre.validate_rows(rows)
   for r in rows:
    b=baseline[r['node_id']]
    for k in ('digest','peer_id','pin_sha256','binary_sha256','genesis'):
     if r[k]!=b[k]:raise ValueError('measurement/live baseline differs: '+k)
    if r['protocol'].get('apow'):raise ValueError('APoW already configured')
   height=min(r['height'] for r in rows);proofs=batch('commitment',height=height);pre.compare(proofs,height)
   # Live plan, never a height copied from the earlier measurement.
   plan={'schema':1,'chain_id':4735490,'binary_sha256':ns['BIN_SHA'],'activation':dict(ns['PROFILE'],activation_height=max(r['height'] for r in rows)+3000),'nodes':rows,'common_height':height,'commitments':proofs}
   ns['validate_plan'](plan)
   with planpath.open('x') as f:json.dump(plan,f,indent=2);f.write('\n')
   planpath.chmod(0o600);print('PLAN_WRITTEN_NO_SERVICE_CHANGED activation_height='+str(plan['activation']['activation_height']));return
  if args.action=='migrate':
   persist('prepare',batch('prepare'));print('FIVE_PREPARED',flush=True)
   persist('stop',batch('stop'));print('FIVE_STOP_REQUESTS',flush=True)
   deadline=time.monotonic()+180
   while True:
    try:persist('snapshot',batch('snapshot'));break
    except Exception as e:
     if any(s.broken for s in sessions) or time.monotonic()>deadline:raise
     print('STOP_WAIT '+str(e),flush=True);time.sleep(5)
   persist('launch',batch('launch',worker_source=worker_source));print('FIVE_PERSISTENT_MIGRATION_JOBS_REQUESTED',flush=True)
   # Explicitly finish this invocation. The Mac can disconnect now.
   print('NEXT: coordinator.py jobs --plan '+str(planpath));return
  if args.action=='jobs':
   deadline=time.monotonic()+args.timeout
   while True:
    rows=batch('job');persist('jobs',rows)
    if any(r.get('not_submitted') for r in rows):raise ValueError('some jobs were not submitted; resume migrate with the SAME plan')
    if any(r.get('lost_job_after_intent') for r in rows):raise ValueError('job disappeared after its launch intent; inspect systemd and remote logs before retrying')
    if any(r.get('file')=='FAILED.json' or r.get('job',{}).get('ActiveState')=='failed' for r in rows):raise ValueError('offline job failed; inspect remote migration.log; no start')
    if all(r.get('file')=='DONE.json' for r in rows):
     persist('offline-gates',batch('gate'));print('FIVE_OFFLINE_MIGRATIONS_VERIFIED_NO_NODE_STARTED');return
    if time.monotonic()>deadline:raise TimeoutError('jobs retained; repeat jobs, not migration')
    print('OFFLINE_JOBS_DONE='+str(sum(r.get('file')=='DONE.json' for r in rows))+'/5',flush=True);time.sleep(20)
  if args.action=='start':
   # Every remote must independently pass before ANY start is authorized.
   persist('offline-gates',batch('start-gate'));persist('install',batch('install'));persist('start',batch('start'))
   print('FIVE_NODE_START_REQUESTS_MINERS_NOT_STARTED');return
  if args.action=='closeout':
   rows=batch('status');pre.validate_rows(rows);h=min(r['height'] for r in rows)
   proofs=batch('commitment',height=h);pre.compare(proofs,h)
   recovery=sessions[3].call({'action':'restart-check','node_id':'hvm-testnet-v4','plan':plan,'_timeout':50})
   results=batch('finalize');persist('closeout',{'agreement':proofs,'recovery':recovery,'services':results,'mainnet_activated':False})
   print('APOW_TESTNET_ACCEPTANCE_COMPLETE_MAINNET_NOT_ACTIVATED');return
  if args.action=='miners':
   rows=batch('status');pre.validate_rows(rows)
   for r in rows:
    if r['binary_sha256']!=ns['BIN_SHA'] or r['protocol'].get('apow')!=plan['activation']:raise ValueError('five migrated ready nodes required')
   h=min(r['height'] for r in rows);pre.compare(batch('commitment',height=h),h)
   miner=module('miner-service');sha='32e62dc2a41d16c0ea472f5aa69e53f0a752cc13c1c9ca61c5e25af7a9998cdc'
   text=miner.unit(Path('/opt/hashburst-apow-miner')/sha/'hvm-apow-miner')
   results=[]
   for s,(_,node) in zip(sessions[:4],pre.TARGETS[:4]):results.append(s.call({'action':'miner-start','node_id':node,'plan':plan,'miner_sha256':sha,'unit_text':text,'_timeout':50}))
   persist('miners',results);print('FOUR_MINERS_STARTED_BY_EXPLICIT_OPERATOR_COMMAND');return
  if args.action=='audit':
   binary=R/'hvm-apow-audit';expected=hashlib.sha256(binary.read_bytes()).hexdigest()
   declared=json.loads((R/'audit-release.json').read_text())['sha256']
   if expected!=declared:raise ValueError('audit binary checksum')
   uploader=module('stage-all')
   for s,(_,node) in zip(sessions,pre.TARGETS):
    remote=s.call({'action':'prepare-upload'})
    uploader.upload_file(s,remote,binary)
    s.call({'action':'install-audit','node_id':node,'plan':plan,'upload':remote,'sha256':expected,'_timeout':50})
   # Two full retarget windows, from the first APoW height. Read-only.
   proofs=[]
   for s,(_,node) in zip(sessions,pre.TARGETS):
    print('AUDIT='+node,flush=True)
    proofs.append(s.call({'action':'audit','node_id':node,'plan':plan,'height':plan['activation']['activation_height'],'count':64,'_timeout':600}))
   for i in range(64):
    pre.compare([p[i]['commitment'] for p in proofs],plan['activation']['activation_height']+i)
    if len({json.dumps({k:v for k,v in p[i]['audit'].items() if k!='certificate'},sort_keys=True) for p in proofs})!=1:raise ValueError('reward proof differs between nodes')
   persist('rewards',proofs);print('FIVE_NODE_64_FINALIZED_APOW_REWARDS_OK_50_HBT_EACH');return
  if args.action=='restart':
   rows=batch('status');pre.validate_rows(rows);h=min(r['height'] for r in rows)
   if h<plan['activation']['activation_height']+64:raise ValueError('two APoW windows must finalize before restart')
   proofs=batch('commitment',height=h);pre.compare(proofs,h);persist('before-restart',proofs)
   result=sessions[3].call({'action':'restart-proof','node_id':'hvm-testnet-v4','plan':plan,'_timeout':50});persist('restart',result);print('V4_RESTART_REQUESTED_ONCE');return
  deadline=time.monotonic()+args.timeout;first=None
  while True:
   try:
    rows=batch('status');pre.validate_rows(rows)
    for r in rows:
     if r['binary_sha256']!=ns['BIN_SHA'] or r['protocol'].get('apow')!=plan['activation']:raise ValueError('wrong runtime/profile')
    if first is None:first={r['node_id']:r['height'] for r in rows}
    h=min(r['height'] for r in rows);proofs=batch('commitment',height=h);pre.compare(proofs,h)
    if args.action=='restart-check':
     result=sessions[3].call({'action':'restart-check','node_id':'hvm-testnet-v4','plan':plan,'_timeout':50});persist('restart-check',result)
    waiting_for_work=args.action=='verify' and all(r['height']==plan['activation']['activation_height']-1 for r in rows)
    if waiting_for_work or all(r['height']>first[r['node_id']] for r in rows):
     persist('agreement',proofs);persist('progress',rows);print(('FIVE_NODE_READY_AT_ACTIVATION_BARRIER height=' if waiting_for_work else 'FIVE_NODE_AGREEMENT_AND_PROGRESS_OK height=')+str(h));print('REWARD_AUDIT_REQUIRED' if h>=plan['activation']['activation_height'] else 'MINERS_MAY_BE_STARTED_MANUALLY');return
   except Exception as e:
    if any(s.broken for s in sessions):raise
    print('READINESS_WAIT '+str(e),flush=True)
   if time.monotonic()>deadline:raise TimeoutError('readiness timeout; services retained; repeat verify')
   time.sleep(15)
 finally:
  for s in sessions:s.close()
if __name__=='__main__':
 try:main()
 except Exception as e:raise SystemExit('STOP: '+str(e)+'; state, journals and persistent jobs retained; no automatic rollback')
