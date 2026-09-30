"""Stage immutable binaries and a stopped miner; called over dedicated SSH."""
import hashlib, json, os, platform, shutil, subprocess
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
def main(request):
 if request['action'] in ('status','commitment'):return inspect(request)
 if request['action']=='prepare-upload':
  import tempfile
  return tempfile.mkdtemp(prefix='hvm-apow-stage-',dir='/root')
 if request['action']=='stage':return stage(request)
 raise ValueError('unknown action')
