#!/usr/bin/env python3
"""Rolling startup-order correction. No state migration or consensus reset."""
import argparse,base64,concurrent.futures,hashlib,importlib.util,json,sys,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ssh_session',R/'ssh-session.py');ssh=importlib.util.module_from_spec(spec);spec.loader.exec_module(ssh)
OLD='1fcdb11df7a23d06a22a7aa56cb3fc5034088129cad6f098a6c2bf32a54e988f'
NODES=[{'ip':ip,'node_id':'hvm-testnet-v'+str(i+1),'role':'validator'} for i,ip in enumerate(('77.90.188.153','77.90.188.154','77.90.188.155','77.90.188.157'))]+[{'ip':'64.31.4.9','node_id':'hvm-testnet-ingress','role':'observer'}]
FIELDS=('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root','evm_state_root','evm_receipts_root','evm_gas_used')
def compare(rows,height):
 for r in rows:
  if r.get('chain_id')!=4735490 or r.get('height')!=height or not r.get('certificate'):raise RuntimeError('invalid finalized commitment')
  if height>=53303 and (not r.get('evm_state_root') or not r.get('evm_receipts_root')):raise RuntimeError('missing EVM commitments')
 if any(tuple(r.get(k) for k in FIELDS)!=tuple(rows[0].get(k) for k in FIELDS) for r in rows[1:]):raise RuntimeError('same-height commitments disagree')
def ready(rows):
 return all(r.get('reactor_running') and r.get('peer_count',0)>=2 and r.get('chain_id')==4735490 for r in rows)
def retryable_read_error(exc):
 # Batch errors retain the node prefix. Retry only the two observed read errors;
 # identity, agreement, certificate, SSH and release failures remain terminal.
 parts=str(exc).split('; ')
 return bool(parts) and all(part.endswith('timed out') or part.endswith('<urlopen error [Errno 111] Connection refused>') for part in parts)
def main():
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=('apply','verify','restart-v4'));a=parser.parse_args()
 for line in (R/'SHA256SUMS').read_text().splitlines():
  digest,name=line.split('  ',1)
  if hashlib.sha256((R/name).read_bytes()).hexdigest()!=digest:raise RuntimeError('checksum '+name)
 raw=(R/'hashburst-testnet').read_bytes();sha=hashlib.sha256(raw).hexdigest();source=(R/'node.py').read_text()
 logs=Path(tempfile.mkdtemp(prefix='startup-results-',dir=R));sessions={}
 def call(n,action,**kw):
  try:
   r=sessions[n['ip']].call({'node':n,'action':action,'sha256':sha,'_timeout':180,**kw})
   (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'.json')).write_text(json.dumps(r,indent=2));return r
  except Exception as e:
   (logs/(str(time.time_ns())+'-'+n['node_id']+'-'+action+'-error.txt')).write_text(str(e));raise
 def batch(nodes,action,**kw):
  with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
   jobs=[pool.submit(call,n,action,**kw) for n in nodes];rows=[];errors=[]
   for n,f in zip(nodes,jobs):
    try:rows.append(f.result())
    except Exception as e:errors.append(n['node_id']+': '+str(e))
   if errors:raise RuntimeError('; '.join(errors))
   return rows
 def agreement(nodes):
  rows=batch(nodes,'status')
  if not ready(rows):raise RuntimeError('reactor/peers not ready')
  if len({r['config_digest'] for r in rows})!=1:raise RuntimeError('config digests differ')
  height=min(r['finalized_height'] for r in rows);compare(batch(nodes,'proof',height=height),height);return height
 def wait_catchup():
  deadline=time.monotonic()+7200
  while time.monotonic()<deadline:
   try:
    rows=batch(NODES,'status')
    heights=[r['finalized_height'] for r in rows]
    print('CATCHUP '+json.dumps({n['node_id']:r['finalized_height'] for n,r in zip(NODES,rows)}),flush=True)
    if ready(rows) and max(heights)-min(heights)<=8:
     height=agreement(NODES);print('FIVE_NODE_CATCHUP_OK='+str(height),flush=True);return
   except Exception as exc:
    if any(s.broken for s in sessions.values()) or not retryable_read_error(exc):raise
    print('READ_ONLY_RETRY '+str(exc),flush=True)
   time.sleep(15)
  raise RuntimeError('bounded catch-up timeout; next node not stopped')
 def wait_node(n):
  deadline=time.monotonic()+7200;report=0
  while time.monotonic()<deadline:
   try:
    r=call(n,'status')
    if ready([r]):call(n,'release');print('RECOVERY_READY='+n['node_id'],flush=True);return
    why='reactor/peers not ready'
   except Exception as e:
    if sessions[n['ip']].broken:raise RuntimeError('SSH lost; services retained; reconnect with apply to skip completed nodes') from e
    why=str(e)
   if time.monotonic()-report>=60:
    d=call(n,'diagnostic');print('RUNTIME '+n['node_id']+' '+json.dumps(d),flush=True)
    if d['systemd']['ActiveState']=='failed' or d['systemd']['SubState']=='auto-restart' or int(d['systemd']['NRestarts'])>0:raise RuntimeError('runtime exited/restarted; inspect diagnostics before resuming')
    report=time.monotonic()
   print('REPLAY_WAIT '+n['node_id']+' '+why,flush=True);time.sleep(15)
  raise RuntimeError('bounded startup wait expired; state retained')
 try:
  for n in NODES:
   print('SSH_AUTH='+n['ip'],flush=True);sessions[n['ip']]=ssh.Session(ssh.ssh_command(n['ip']),source)
  if a.action=='apply':
   for n in NODES:
    info=call(n,'inspect')
    if info['sha256']==sha:
     call(n,'prepared');print('ALREADY_UPDATED_NO_RESTART='+n['node_id'],flush=True);wait_node(n);wait_catchup();continue
    if info['sha256']!=OLD or info['prepared']:raise RuntimeError('partial/unexpected release on '+n['node_id']+'; inspect before further mutation')
    others=[v for v in NODES[:4] if v!=n]
    baseline=agreement(others);time.sleep(5)
    if agreement(others)<=baseline:raise RuntimeError('other validators not advancing; no node stopped')
    print('UPGRADING='+n['node_id'],flush=True)
    call(n,'stage',binary=base64.b64encode(raw).decode())
    call(n,'stop');call(n,'install');call(n,'start');wait_node(n)
    wait_catchup()
  elif a.action=='restart-v4':
   gate=json.loads((R/'GATE-startup.json').read_text())
   if gate.get('binary_sha256')!=sha or gate.get('height',0)<53303 or time.time()-gate['timestamp']>86400:raise RuntimeError('recent finality gate required')
   batch(NODES,'release');call(NODES[3],'restart');wait_node(NODES[3])
  else:
   for n in NODES:wait_node(n)
  deadline=time.monotonic()+7200;last=None;changed=time.monotonic();activated=None
  while time.monotonic()<deadline:
   try:height=agreement(NODES)
   except Exception as exc:
    if any(s.broken for s in sessions.values()) or not retryable_read_error(exc):raise
    if time.monotonic()-changed>600:raise RuntimeError('no verified progress for 600 seconds; services retained') from exc
    print('READ_ONLY_RETRY '+str(exc),flush=True);time.sleep(15);continue
   print('COMMON_FINALITY='+str(height),flush=True)
   if last is not None and height<last:raise RuntimeError('finalized height regressed')
   if last is None or height>last:last=height;changed=time.monotonic()
   if time.monotonic()-changed>600:raise RuntimeError('finality stalled; services retained')
   if height>=53303:
    if activated is None:activated=height
    elif height>=activated+5:
     batch(NODES,'release')
     if a.action=='restart-v4':call(NODES[3],'restart-check');print('V4_PERSISTENT_RESTART_OK',flush=True)
     (R/'GATE-startup.json').write_text(json.dumps({'ok':True,'chain_id':4735490,'binary_sha256':sha,'height':height,'logs':str(logs),'timestamp':time.time()},indent=2))
     print('HVM_TESTNET_STARTUP_AND_EVM_FINALITY_OK',flush=True);return
   time.sleep(15)
  raise RuntimeError('bounded finality wait expired; use verify')
 finally:
  for s in sessions.values():s.close()
  print('LOGS='+str(logs),flush=True)
if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:print('STOP: state and journals retained: '+str(e));sys.exit(1)
