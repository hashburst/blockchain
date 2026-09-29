#!/usr/bin/env python3
"""Offline migration worker. Owned by systemd, independent of operator SSH."""
import hashlib,json,os,subprocess,sys,time
from pathlib import Path
ORIGINAL='69d9b71dacf2fbf51843789ac3c073a946b174022e7fcbc5cbd80a6ed11db298'
FILES=('blockchain.dat','blockchain.idx','consensus-votes.jsonl','consensus-bft-signatures.jsonl','consensus-recovery.json')
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb') as f:
  for b in iter(lambda:f.read(1048576),b''):h.update(b)
 return h.hexdigest()
def atomic(p,obj):
 p=Path(p);tmp=p.with_name(p.name+'.tmp')
 with tmp.open('w') as f:json.dump(obj,f,indent=2);f.flush();os.fsync(f.fileno())
 os.replace(tmp,p)
 fd=os.open(p.parent,os.O_RDONLY)
 try:os.fsync(fd)
 finally:os.close(fd)
def inventory(data):return {n:{'size':(data/n).stat().st_size,'sha256':sha(data/n)} for n in FILES if (data/n).exists()}
def validate_proof(c,proof,job):
 if c['protocol']['chain_id']!=4735490 or c['node_id']!=job['node_id'] or c['role']!=job['role']:raise RuntimeError('identity/network mismatch')
 if c['protocol'].get('evm') not in (None,job['evm']):raise RuntimeError('different activation parameters')
 if proof['binary_sha256']!=ORIGINAL or proof['chain_id']!=4735490 or proof['evm']!=job['evm']:raise RuntimeError('original rollout evidence mismatch')
 if proof['identity']!=c['peer_id'] or proof['genesis']!=c['genesis_hash']:raise RuntimeError('original identity mismatch')
 if c['role']=='observer' and c.get('consensus_key_file'):raise RuntimeError('observer signing key configured')
def main(jobpath):
 j=json.loads(Path(jobpath).read_text());root=Path(jobpath).parent;cfg=Path(j['config']);c=json.loads(cfg.read_text());data=Path(c['data_dir'])
 proof=json.loads((data/'evm-rollout-proof.json').read_text());validate_proof(c,proof,j)
 if sha(j['binary'])!=j['sha256']:raise RuntimeError('binary checksum mismatch')
 state=subprocess.check_output(['systemctl','show',j['service'],'-p','ActiveState','--value'],text=True).strip()
 if state not in ('inactive','failed'):raise RuntimeError('testnet must remain stopped during offline migration')
 for name,v in proof['journals'].items():
  if (data/name).stat().st_size!=v['length'] or sha(data/name)!=v['sha256']:raise RuntimeError('journal changed since original stop barrier')
  if c['role']=='observer' and v['length']:raise RuntimeError('observer signed')
 before=inventory(data);record=root/'before.json'
 if record.exists():
  if json.loads(record.read_text())!=before:raise RuntimeError('state differs from previous offline attempt')
 else:atomic(record,before)
 candidate=root/'candidate.json';c['protocol']['evm']=j['evm'];atomic(candidate,c);candidate.chmod(0o600)
 for phase,args in (
  ('migration',['--config',str(cfg),'--migrate-evm',str(candidate)]),
  ('validation',['--config',str(cfg),'--check'])):
  atomic(root/'progress.json',{'phase':phase,'started':time.time(),'pid':os.getpid()});print('OFFLINE_PHASE='+phase,flush=True)
  child=subprocess.Popen([j['binary']]+args)
  atomic(root/'progress.json',{'phase':phase,'started':time.time(),'pid':os.getpid(),'child_pid':child.pid})
  code=child.wait()
  if code:raise RuntimeError('runtime '+phase+' exited '+str(code))
 if inventory(data)!=before:raise RuntimeError('chain, journal or recovery snapshot changed during offline migration')
 drop=Path('/etc/systemd/system')/(j['service']+'.d');drop.mkdir(exist_ok=True)
 contents='[Service]\nExecStart=\nExecStart='+j['binary']+' --config '+str(cfg)+'\n'
 target=drop/'50-evm-release.conf'
 if target.exists() and not (root/'previous-drop-in.conf').exists():
  (root/'previous-drop-in.conf').write_bytes(target.read_bytes())
 target.write_text(contents);subprocess.run(['systemctl','daemon-reload'],check=True)
 atomic(root/'result.json',{'ok':True,'binary_sha256':j['sha256'],'node_id':j['node_id'],'evm':j['evm'],'preserved':before})
 print('OFFLINE_MIGRATION_AND_RECOVERY_OK='+j['node_id'],flush=True)
if __name__=='__main__':
 try:main(sys.argv[1])
 except BaseException as e:
  print('OFFLINE_FAILED: '+str(e),flush=True)
  raise
