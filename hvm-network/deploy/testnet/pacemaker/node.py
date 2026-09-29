"""Coordinated binary-only testnet update. No migration or journal replacement."""
import base64,hashlib,json,os,pwd,subprocess,time,urllib.request
from pathlib import Path
OLD='d91ed8eeef0fb9290f74516883d536caf52500878ef89099a81f1a4b6064c8d3'
DIGEST='71f6c677b00f42d181a2dc4fa5b1c3dbe50101a2599cac80357aed7fbf8b1021'
EVM={'activation_height':53303,'gas_limit':200000,'base_fee_wei':1}
JOURNALS=('consensus-votes.jsonl','consensus-bft-signatures.jsonl')
def require(ok,msg):
 if not ok:raise RuntimeError(msg)
def sha(path,length=None):
 h=hashlib.sha256()
 with Path(path).open('rb') as f:
  left=length
  while left is None or left:
   b=f.read(1048576 if left is None else min(left,1048576))
   if not b:break
   h.update(b)
   if left is not None:left-=len(b)
  require(not left,'shortened journal')
 return h.hexdigest()
def command(*args):
 return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True,timeout=60).strip()
def get(path,payload=None):
 req=urllib.request.Request('http://127.0.0.1:18009'+path,None if payload is None else json.dumps(payload).encode(),{'Content-Type':'application/json'})
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=15) as f:return json.load(f)
def atomic(path,obj):
 tmp=path.with_suffix('.tmp');fd=os.open(tmp,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'w') as f:json.dump(obj,f);f.flush();os.fsync(f.fileno())
 os.replace(tmp,path)
 fd=os.open(path.parent,os.O_RDONLY)
 try:os.fsync(fd)
 finally:os.close(fd)
def trusted(path):
 p=Path(path)
 for q in (p,*p.parents):
  st=q.lstat();require(not q.is_symlink() and st.st_uid==0 and not st.st_mode&0o022,'untrusted release path '+str(q))
def main(p):
 n=p['node'];observer=n['role']=='observer';prefix='hashburst-hvm-testnet'+('-ingress' if observer else '')
 cfg=Path('/etc')/prefix/'node.json';c=json.loads(cfg.read_text());data=Path(c['data_dir']);unit=prefix+'.service'
 require(c['node_id']==n['node_id'] and c['role']==n['role'] and c['protocol']['chain_id']==4735490,'identity mismatch')
 require(c['protocol'].get('evm')==EVM,'activation schedule differs')
 require(not observer or not c.get('consensus_key_file'),'observer signing configured')
 root=Path('/opt')/prefix/('pacemaker-'+p['sha256']);binary=root/'hashburst-testnet';record=root/'preservation.json'
 action=p['action']
 def props():return dict(line.split('=',1) for line in command('systemctl','show',unit,'-p','MainPID','-p','ActiveState','-p','SubState','-p','User','-p','ExecMainStatus','-p','NRestarts').splitlines())
 def running_hash():
  pid=int(props()['MainPID']);return sha('/proc/'+str(pid)+'/exe') if pid else None
 def preservation():
  old=json.loads((data/'evm-rollout-proof.json').read_text())
  require(old['chain_id']==4735490 and old['evm']==EVM,'original network proof differs')
  require(old['identity']==c['peer_id'] and old['genesis']==c['genesis_hash'],'original identity changed')
  for name,v in old['journals'].items():require(sha(data/name,v['length'])==v['sha256'],'original journal prefix changed')
  if observer:
   for name in JOURNALS:require((data/name).stat().st_size==0,'observer signed')
  if record.exists():
   saved=json.loads(record.read_text());require(saved['binary_sha256']==p['sha256'],'different update proof')
   require(sha(cfg)==saved['config'] and sha(data/'runtime.pin')==saved['pin'],'config/pin changed')
   for name,v in saved['journals'].items():require(sha(data/name,v['length'])==v['sha256'],'update journal prefix changed')
  return {'preserved':True,'observer_unsigned':observer}
 if action=='status':
  h=get('/health');require(h['node_id']==n['node_id'] and h['chain_id']==4735490 and h['config_digest']==DIGEST,'health identity/config mismatch');return h
 if action=='proof':
  r=get('/rpc',{'jsonrpc':'2.0','id':1,'method':'hb_getFinalizedCommitment','params':[p['height']]});require('error' not in r,str(r));return r['result']
 if action=='diagnostic':return {'systemd':props(),'log':command('journalctl','-u',unit,'-n','12','--no-pager','-o','cat')}
 if action=='preservation':return preservation()
 if action=='stage':
  preservation();h=get('/health');require(h.get('config_digest')==DIGEST and h.get('node_id')==n['node_id'],'live config differs')
  require(running_hash()==OLD,'expected old runtime not running; use verify for already-updated nodes')
  root.mkdir(mode=0o755,parents=True,exist_ok=True);trusted(root);os.chmod(root,0o755)
  raw=base64.b64decode(p['binary'],validate=True);require(hashlib.sha256(raw).hexdigest()==p['sha256'],'binary checksum')
  if binary.exists():require(sha(binary)==p['sha256'],'immutable release differs')
  else:
   fd=os.open(binary,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o755)
   with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
  os.chmod(binary,0o755);trusted(binary)
  user=props()['User'];require(user in ('hashburst-hvm-testnet','hashburst-hvm-ingress'),'service identity')
  account=pwd.getpwnam(user)
  def demote():os.setgroups([]);os.setgid(account.pw_gid);os.setuid(account.pw_uid)
  subprocess.run([str(binary),'--help'],preexec_fn=demote,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE,check=True,timeout=20)
  return {'staged':p['sha256']}
 if action=='stop':
  require(sha(binary)==p['sha256'] and running_hash()==OLD,'release/process mismatch before stop')
  command('systemctl','stop',unit);require(int(props()['MainPID'])==0,'runtime still running')
  preservation()
  saved={'binary_sha256':p['sha256'],'config':sha(cfg),'pin':sha(data/'runtime.pin'),'journals':{name:{'length':(data/name).stat().st_size,'sha256':sha(data/name)} for name in JOURNALS},'files':{name:sha(data/name) for name in ('blockchain.dat','blockchain.idx','consensus-recovery.json') if (data/name).exists()}}
  require(not record.exists(),'existing update evidence retained');atomic(record,saved)
  return {'stopped_and_preserved':True}
 if action=='install':
  require(int(props()['MainPID'])==0,'stop barrier required');saved=json.loads(record.read_text());preservation()
  for name,v in saved['files'].items():require(sha(data/name)==v,'stopped state changed')
  require(sha(binary)==p['sha256'],'binary mismatch');trusted(binary)
  drop=Path('/etc/systemd/system')/(unit+'.d')/'50-evm-release.conf';trusted(drop)
  backup=root/'previous-drop-in.conf'
  if not backup.exists():backup.write_bytes(drop.read_bytes());backup.chmod(0o600)
  drop.write_text('[Service]\nExecStart=\nExecStart='+str(binary)+' --config '+str(cfg)+'\n');command('systemctl','daemon-reload')
  return {'installed':True,'migration_executed':False}
 if action=='prepared':
  preservation();require(record.exists() and sha(binary)==p['sha256'],'prepared release proof missing')
  expected='ExecStart='+str(binary)+' --config '+str(cfg)
  require(expected in (Path('/etc/systemd/system')/(unit+'.d')/'50-evm-release.conf').read_text(),'override mismatch')
  return {'prepared':True}
 if action=='restart':
  require(n['node_id']=='hvm-testnet-v4' and running_hash()==p['sha256'],'v4 verified runtime required')
  marker=root/'restart.json'
  if marker.exists():return {'already_requested':True}
  require(get('/health')['finalized_height']>=53303,'activation not finalized')
  command('systemctl','stop',unit);preservation()
  journal=data/'consensus-bft-signatures.jsonl'
  rows=[json.loads(line) for line in journal.read_text().splitlines() if line.strip()]
  atomic(marker,{'length':journal.stat().st_size,'sha256':sha(journal),'height':max(r['height'] for r in rows),'pin':sha(data/'runtime.pin')})
  command('systemctl','start','--no-block',unit);return {'restart_requested':True}
 if action=='restart-check':
  marker=json.loads((root/'restart.json').read_text());journal=data/'consensus-bft-signatures.jsonl'
  require(sha(journal,marker['length'])==marker['sha256'] and sha(data/'runtime.pin')==marker['pin'],'restart prefix/pin differs')
  with journal.open('rb') as f:f.seek(marker['length']);tail=f.read()
  lines=tail.splitlines()
  if tail and not tail.endswith(b'\n'):lines=lines[:-1]
  rows=[json.loads(line) for line in lines if line.strip()]
  require(any(r.get('step')=='PRECOMMIT' and r.get('chain_id')==4735490 and r.get('validator_id','').lower()==c['validator_id'].lower() and r['height']>marker['height'] for r in rows),'new local precommit not yet observed')
  return {'restart_prefix_preserved':True,'new_precommit_observed':True}
 if action=='start':
  preservation();require(record.exists(),'stop proof required')
  expected='ExecStart='+str(binary)+' --config '+str(cfg)
  require(expected in (Path('/etc/systemd/system')/(unit+'.d')/'50-evm-release.conf').read_text(),'override mismatch')
  command('systemctl','start','--no-block',unit);return {'start_requested':True}
 if action=='release':require(running_hash()==p['sha256'],'running binary mismatch');return preservation()
 raise RuntimeError('unknown action')
