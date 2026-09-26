"""Remote actions for dedicated SSH. No private key is returned or replaced."""
import base64,hashlib,json,os,subprocess,time,urllib.request
from pathlib import Path
NAMES=('consensus-votes.jsonl','consensus-bft-signatures.jsonl')
def run(*args,timeout=3700):
 p=subprocess.run(args,text=True,capture_output=True,timeout=timeout)
 if p.returncode:raise RuntimeError(' '.join(args[:2])+': '+p.stderr[-2000:])
 return p.stdout.strip()
def digest(path,length=None):
 h=hashlib.sha256()
 with open(path,'rb') as f:
  remaining=length
  while remaining is None or remaining:
   b=f.read(1048576 if remaining is None else min(1048576,remaining))
   if not b:break
   h.update(b)
   if remaining is not None:remaining-=len(b)
  if remaining:raise RuntimeError('journal prefix shortened')
 return h.hexdigest()
def get(path,data=None):
 req=urllib.request.Request('http://127.0.0.1:18009'+path,None if data is None else json.dumps(data).encode(),{'Content-Type':'application/json'})
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=20) as r:return json.load(r)
def rpc(method,params):
 d=get('/rpc',{'jsonrpc':'2.0','id':1,'method':method,'params':params})
 if 'error' in d:raise RuntimeError(str(d['error']))
 return d['result']
def main(p):
 n=p['node'];observer=n['role']=='observer';prefix='hashburst-hvm-testnet'+('-ingress' if observer else '')
 cfg=Path('/etc')/prefix/'node.json';c=json.loads(cfg.read_text());data=Path(c['data_dir']);service=prefix+'.service'
 if c['node_id']!=n['node_id'] or c['role']!=n['role'] or c['protocol']['chain_id']!=4735490:raise RuntimeError('node identity/network mismatch')
 if observer and c.get('consensus_key_file'):raise RuntimeError('observer has signing key')
 action=p['action'];root=Path('/opt')/prefix/'evm-release';proof=data/'evm-rollout-proof.json'
 if action=='status':
  h=get('/health')
  if h['chain_id']!=4735490 or h['node_id']!=n['node_id'] or h['role']!=n['role']:raise RuntimeError('health identity mismatch')
  return h
 if action=='proof':return rpc('hb_getFinalizedCommitment',[p['height']])
 if action=='stage':
  raw=base64.b64decode(p['binary'],validate=True);sha=hashlib.sha256(raw).hexdigest()
  if sha!=p['sha256']:raise RuntimeError('binary digest')
  root.mkdir(mode=0o755,parents=True,exist_ok=True);binary=root/('hashburst-testnet-'+sha)
  if binary.exists():
   if digest(binary)!=sha:raise RuntimeError('staged binary differs')
  else:
   with open(binary,'xb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
   binary.chmod(0o755)
  return {'staged':sha,'binary':str(binary)}
 if action=='stop':
  run('systemctl','stop',service)
  if run('systemctl','show',service,'--property=ActiveState','--value') not in ('inactive','failed'):raise RuntimeError('service not stopped')
  return {'stopped':True}
 if action=='migrate':
  binary=root/('hashburst-testnet-'+p['sha256'])
  if digest(binary)!=p['sha256']:raise RuntimeError('binary digest')
  if run('systemctl','show',service,'--property=ActiveState','--value') not in ('inactive','failed'):raise RuntimeError('stop barrier required')
  evm=p['evm'];candidate=root/'candidate.json'
  if c['protocol'].get('evm') not in (None,evm):raise RuntimeError('different EVM parameters')
  if not proof.exists():
   record={'identity':c['peer_id'],'genesis':c['genesis_hash'],'chain_id':4735490,'binary_sha256':p['sha256'],'evm':evm,'journals':{name:{'length':(data/name).stat().st_size,'sha256':digest(data/name)} for name in NAMES}}
   with open(proof,'x') as f:json.dump(record,f,indent=2);f.flush();os.fsync(f.fileno())
  else:
   record=json.loads(proof.read_text())
   if record['evm']!=evm or record['binary_sha256']!=p['sha256']:raise RuntimeError('rollout proof differs')
  c['protocol']['evm']=evm
  candidate.write_text(json.dumps(c,indent=2));candidate.chmod(0o600)
  run(str(binary),'--config',str(cfg),'--migrate-evm',str(candidate))
  run(str(binary),'--config',str(cfg),'--check')
  # ExecStart only; retain the existing service's identity and hardening.
  drop=Path('/etc/systemd/system')/(service+'.d');drop.mkdir(exist_ok=True)
  (drop/'50-evm-release.conf').write_text('[Service]\nExecStart=\nExecStart='+str(binary)+' --config '+str(cfg)+'\n')
  run('systemctl','daemon-reload');return {'migrated':True,'evidence':str(proof)}
 if action=='start':run('systemctl','start',service);return {'started':True}
 if action=='restart':
  marker=data/'evm-restart-proof.json'
  if marker.exists():raise RuntimeError('restart already requested; poll recovery instead')
  run('systemctl','stop',service)
  journal=data/'consensus-bft-signatures.jsonl';raw=journal.read_bytes()
  records=[json.loads(line) for line in raw.splitlines() if line.strip()]
  if not records:raise RuntimeError('empty validator signing journal')
  record={'length':len(raw),'sha256':hashlib.sha256(raw).hexdigest(),'max_height':max(r['height'] for r in records),'pin_sha256':digest(data/'runtime.pin')}
  with open(marker,'x') as f:json.dump(record,f);f.flush();os.fsync(f.fileno())
  run('systemctl','start',service);return {'restarted':n['node_id']}
 if action=='recovery':
  marker=json.loads((data/'evm-restart-proof.json').read_text());journal=data/'consensus-bft-signatures.jsonl'
  if digest(journal,marker['length'])!=marker['sha256'] or digest(data/'runtime.pin')!=marker['pin_sha256']:raise RuntimeError('restart identity/journal changed')
  with open(journal,'rb') as f:f.seek(marker['length']);tail=f.read()
  lines=tail.splitlines()
  if tail and not tail.endswith(b'\n'):lines=lines[:-1]
  rows=[json.loads(line) for line in lines if line.strip()]
  if not any(r.get('step')=='PRECOMMIT' and r.get('chain_id')==4735490 and r.get('validator_id','').lower()==c['validator_id'].lower() and r['height']>marker['max_height'] for r in rows):raise RuntimeError('new precommit not observed yet')
  return {'restart_journal_prefix_preserved':True,'new_precommit_observed':True,'pin_preserved':True}
 if action=='preservation':
  record=json.loads(proof.read_text())
  if record['identity']!=c['peer_id'] or record['genesis']!=c['genesis_hash']:raise RuntimeError('identity changed')
  for name,v in record['journals'].items():
   if digest(data/name,v['length'])!=v['sha256']:raise RuntimeError('journal prefix changed')
   if observer and (data/name).stat().st_size:raise RuntimeError('observer signed')
  return {'journal_prefixes_preserved':True,'observer_no_signing':observer}
 raise RuntimeError('unknown action')
