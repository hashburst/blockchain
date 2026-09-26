import base64,hashlib,json,os,socket,subprocess,time,urllib.request
from pathlib import Path
UNIT='hashburst-hvm-testnet.service'
DATA=Path('/var/lib/hashburst-hvm-testnet')
DIGEST='502c051c2cf6040b988fe2bc94ae984ab7f709e059918237110af678266ea215'
def require(ok,msg):
 if not ok:raise RuntimeError(msg)
def get(method=None,params=[]):
 url='http://127.0.0.1:18009/'+('rpc' if method else 'health')
 body=None if not method else json.dumps(dict(jsonrpc='2.0',id=1,method=method,params=params)).encode()
 req=urllib.request.Request(url,body,{'Content-Type':'application/json'})
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=8) as r:d=json.load(r)
 if method:
  require('error' not in d,str(d));return d['result']
 return d
def health(n):
 d=get()
 require(d.get('chain_id')==4735490 and d.get('config_digest')==DIGEST and d.get('node_id')==n['node_id'] and d.get('role')=='validator','identity/configuration mismatch')
 require(d.get('reactor_running') and d.get('peer_count',0)>=3,'reactor/peers not ready')
 require(d.get('finalized_height',0)>7,'finality not ready')
 return d
def prefix(path,size):
 h=hashlib.sha256()
 with path.open('rb') as f:
  while size:
   b=f.read(min(size,1048576));require(bool(b),'journal truncated');h.update(b);size-=len(b)
 return h.hexdigest()
def run(*args):
 subprocess.run(args,check=True,timeout=120,stdout=subprocess.DEVNULL)
def recovered(n,binary,expected_sha):
 require(binary.is_file() and hashlib.sha256(binary.read_bytes()).hexdigest()==expected_sha,'installed binary mismatch')
 pid=int(subprocess.check_output(['systemctl','show',UNIT,'-p','MainPID','--value']).strip())
 require(pid>0 and Path('/proc/'+str(pid)+'/exe').resolve()==binary.resolve(),'running binary mismatch')
 proofs=list(Path('/root').glob('hvm-api-upgrade-*/proof.json'))
 require(len(proofs)==1,'expected exactly one original upgrade proof; retain state for review')
 proof=json.loads(proofs[0].read_text())
 require(prefix(DATA/'consensus-bft-signatures.jsonl',proof['journal_size'])==proof['journal_sha256'],'journal prefix changed')
 require(hashlib.sha256((DATA/'runtime.pin').read_bytes()).hexdigest()==proof['pin_sha256'],'pin changed')
 h=health(n)
 require(h['finalized_height']>proof['finalized_before'],'finality has not advanced')
 c=get('hb_getFinalizedCommitment',[proof['finalized_before']])
 require(c.get('height')==proof['finalized_before'] and c.get('chain_id')==4735490 and bool(c.get('certificate')),'invalid finalized proof')
 return dict(health=h,journal_prefix_preserved=True,already_updated=True,no_restart=True)
def main(p):
 n=p['node'];require(os.geteuid()==0 and socket.gethostname()==n['hostname'],'wrong host')
 action=p['action']
 if action=='health':return health(n)
 if action=='proof':return get('hb_getFinalizedCommitment',[p['height']])
 require(action=='upgrade','unknown action')
 before=health(n)
 raw=base64.b64decode(p['binary'],validate=True)
 require(hashlib.sha256(raw).hexdigest()==p['sha256'],'binary checksum')
 release=Path('/opt/hashburst-hvm-testnet/releases/f7811f75ca1c1a5269d4f4faed0ee0cdcb2bd516')
 release.mkdir(mode=0o755,parents=True,exist_ok=True)
 binary=release/'hashburst-testnet'
 if binary.exists():require(hashlib.sha256(binary.read_bytes()).hexdigest()==p['sha256'],'release collision')
 else:
  with binary.open('xb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
  binary.chmod(0o755)
 override=Path('/etc/systemd/system/'+UNIT+'.d/30-api-runtime.conf')
 if override.exists():return recovered(n,binary,p['sha256'])
 backup=Path('/root/hvm-api-upgrade-'+str(time.time_ns()));backup.mkdir(mode=0o700)
 (backup/'unit-before.txt').write_bytes(subprocess.check_output(['systemctl','cat',UNIT]))
 pin=(DATA/'runtime.pin').read_bytes()
 # No chain/journal backup is ever restored.
 run('systemctl','stop',UNIT)
 journal=DATA/'consensus-bft-signatures.jsonl';size=journal.stat().st_size
 digest=prefix(journal,size)
 (backup/'proof.json').write_text(json.dumps(dict(journal_size=size,journal_sha256=digest,pin_sha256=hashlib.sha256(pin).hexdigest(),finalized_before=before['finalized_height'])))
 override.parent.mkdir(parents=True,exist_ok=True)
 with override.open('x') as f:
  f.write('[Service]\nExecStart=\nExecStart='+str(binary)+' --config /etc/hashburst-hvm-testnet/node.json\n')
 run('systemctl','daemon-reload');run('systemctl','start',UNIT)
 end=time.monotonic()+3600;last='not ready'
 while time.monotonic()<end:
  try:
   after=health(n)
   if after['finalized_height']>before['finalized_height']:
    require((DATA/'runtime.pin').read_bytes()==pin,'pin changed')
    require(prefix(journal,size)==digest,'journal prefix changed')
    get('hb_getFinalizedCommitment',[before['finalized_height']])
    return dict(health=after,journal_prefix_preserved=True,backup=str(backup))
  except Exception as e:last=str(e)
  time.sleep(3)
 raise RuntimeError('startup/progress timeout; state retained: '+last)
