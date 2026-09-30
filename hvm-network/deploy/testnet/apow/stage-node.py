"""Stage immutable binaries and a stopped miner; called over dedicated SSH."""
import base64, hashlib, json, os, platform, shutil, subprocess, re
from pathlib import Path
TARGETS={'hvm-testnet-v1':'77.90.188.153','hvm-testnet-v2':'77.90.188.154','hvm-testnet-v3':'77.90.188.155','hvm-testnet-v4':'77.90.188.157','hvm-testnet-ingress':'64.31.4.9'}
def digest(path,size=None):
 h=hashlib.sha256()
 with Path(path).open('rb') as f:
  remaining=f.seek(0,2) if size is None else size;f.seek(0)
  while remaining:
   b=f.read(min(1048576,remaining))
   if not b:raise ValueError('protected prefix shortened')
   h.update(b);remaining-=len(b)
 return h.hexdigest()
def protected(config):
 data=Path(config['data_dir']);out={}
 for path in [Path('/etc/hashburst-hvm-testnet'+('-ingress' if config['role']=='observer' else '')+'/node.json'), data/'runtime.pin',data/'consensus-votes.jsonl',data/'consensus-bft-signatures.jsonl']:
  if path.is_symlink():raise ValueError('symlinked protected file')
  if not path.exists():
   if config['role']!='observer' or path.name in ('node.json','runtime.pin'):raise ValueError('missing protected file '+path.name)
   continue
  size=path.stat().st_size;out[str(path)]={'size':size,'sha256':digest(path,size),'exact':path.name in ('node.json','runtime.pin')}
 return out
def verify_protected(proof):
 for p,e in proof.items():
  if Path(p).is_symlink() or not Path(p).is_file():raise ValueError('protected file replaced')
  if digest(p,e['size'])!=e['sha256'] or (e['exact'] and Path(p).stat().st_size!=e['size']):raise ValueError('protected configuration/prefix changed: '+p)
def stage(request):
 node=request['node_id']
 if node not in TARGETS or os.geteuid()!=0 or platform.machine()!='x86_64':raise ValueError('expected root Linux x86_64 testnet host')
 ip=subprocess.check_output(['ip','-4','addr','show'],text=True,timeout=10)
 if 'inet '+TARGETS[node]+'/' not in ip:raise ValueError('wrong host IP')
 before=inspect({'action':'status','node_id':node})
 if before['protocol'].get('apow'):raise ValueError('APoW already configured; use recovery workflow')
 unit='hashburst-hvm-testnet'+('-ingress' if node.endswith('ingress') else '')
 cfg=json.loads((Path('/etc')/unit/'node.json').read_text())
 proofs=protected(cfg)
 upload=Path(request['upload'])
 if not str(upload).startswith('/root/hvm-apow-stage-') or upload.is_symlink():raise ValueError('invalid staging path')
 manifest=json.loads((upload/'stage-manifest.json').read_text())
 if manifest!=request['manifest'] or set(manifest)!={'hashburst-testnet','hvm-apow-miner','miner-service.py','miner-release.json'}:raise ValueError('manifest changed or incomplete')
 for name,h in manifest.items():
  if name not in ('hashburst-testnet','hvm-apow-miner','miner-service.py','miner-release.json'):raise ValueError('unexpected staged file')
  path=upload/name
  if path.is_symlink() or not path.is_file() or digest(path)!=h:raise ValueError('staged checksum '+name)
 dest=Path('/opt/hashburst-apow-candidate')/manifest['hashburst-testnet']
 for p in (dest.parent,dest):
  if p.is_symlink():raise ValueError('symlink candidate directory')
  p.mkdir(mode=0o755,exist_ok=True)
  if p.stat().st_uid!=0 or p.stat().st_mode & 0o022:raise ValueError('unsafe candidate directory permissions')
 binary=dest/'hashburst-testnet'
 if binary.exists():
  if binary.is_symlink() or digest(binary)!=manifest['hashburst-testnet']:raise ValueError('candidate collision')
 else:
  with binary.open('xb') as out, (upload/'hashburst-testnet').open('rb') as src:
   shutil.copyfileobj(src,out);out.flush();os.fsync(out.fileno())
  binary.chmod(0o755)
 miner=None
 if cfg['role']=='validator':
  state=subprocess.run(['systemctl','is-active','hashburst-apow-miner.service'],capture_output=True,text=True,timeout=10).stdout.strip()
  if state in ('active','activating','reloading','deactivating'):raise ValueError('existing miner running; retained')
  miner=subprocess.check_output(['python3',str(upload/'miner-service.py'),'stage'],text=True,stderr=subprocess.STDOUT,timeout=120)
  if 'MINER_STAGED_NO_SERVICE_STARTED_NO_NODE_CONFIG_CHANGED' not in miner:raise ValueError('miner staging not confirmed')
 verify_protected(proofs)
 after=inspect({'action':'status','node_id':node})
 if after['binary_sha256']!=before['binary_sha256'] or after['pin_sha256']!=before['pin_sha256']:raise ValueError('running release or pin changed during staging')
 return {'node_id':node,'candidate_binary':str(binary),'candidate_sha256':manifest['hashburst-testnet'],'running_binary_sha256':after['binary_sha256'],'height':after['height'],'protected_prefixes':proofs,'miner':miner,'service_restarted':False,'apow_activated':False}
# Uploads are confined to fresh directories created by this SSH session.
UPLOAD_DIRS=set()
UPLOADS={}
UPLOAD_NAMES={'hashburst-testnet','hvm-apow-miner','miner-service.py','miner-release.json','stage-manifest.json'}
def transfer(request):
 directory=request['upload'];name=request['name'];action=request['action']
 if directory not in UPLOAD_DIRS or name not in UPLOAD_NAMES:raise ValueError('unregistered upload path')
 root=Path(directory)
 if root.is_symlink() or not root.is_dir():raise ValueError('upload directory replaced')
 path=root/name;part=root/(name+'.part');key=(directory,name)
 if action=='upload-begin':
  size=request['size'];expected=request['sha256']
  if type(size) is not int or not 0<size<=200_000_000 or not re.fullmatch('[0-9a-f]{64}',expected):raise ValueError('invalid upload metadata')
  if key in UPLOADS or path.exists() or path.is_symlink():raise ValueError('upload already exists')
  with part.open('xb'):pass
  part.chmod(0o600)
  UPLOADS[key]={'size':size,'sha256':expected,'offset':0}
  return {'offset':0}
 if key not in UPLOADS:raise ValueError('upload not begun')
 state=UPLOADS[key]
 if part.is_symlink() or not part.is_file() or part.stat().st_size!=state['offset']:raise ValueError('partial upload changed')
 if action=='upload-chunk':
  encoded=request['data']
  if not isinstance(encoded,str) or len(encoded)>350000:raise ValueError('oversized upload chunk')
  data=base64.b64decode(encoded,validate=True)
  if request['offset']!=state['offset'] or not 0<len(data)<=262144 or state['offset']+len(data)>state['size']:raise ValueError('upload offset/length mismatch')
  with part.open('ab') as f:f.write(data)
  state['offset']+=len(data)
  return {'offset':state['offset']}
 if action=='upload-end':
  if state['offset']!=state['size'] or digest(part)!=state['sha256']:raise ValueError('upload size/checksum mismatch')
  with part.open('rb') as f:os.fsync(f.fileno())
  os.link(part,path)  # exclusive publication: never replace another file
  part.unlink();del UPLOADS[key]
  return {'size':path.stat().st_size,'sha256':digest(path)}
 raise ValueError('unknown upload action')
def main(request):
 if request['action'] in ('status','commitment'):return inspect(request)
 if request['action']=='prepare-upload':
  import tempfile
  directory=tempfile.mkdtemp(prefix='hvm-apow-stage-',dir='/root')
  UPLOAD_DIRS.add(directory)
  return directory
 if request['action'].startswith('upload-'):return transfer(request)
 if request['action']=='stage':return stage(request)
 raise ValueError('unknown action')
