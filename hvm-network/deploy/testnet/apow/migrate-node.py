"""Remote APoW lifecycle. Called through the dedicated SSH runner; no key export."""
import copy, hashlib, json, os, re, subprocess, time
from pathlib import Path
BIN_SHA='0e597329261f9376d9c33b1fe427638fd2810bfd43ce5bd702451e785d9f339d'
BIN=Path('/opt/hashburst-apow-candidate')/BIN_SHA/'hashburst-testnet'
PROFILE={'initial_bits':12,'min_bits':8,'max_bits':14,'window':32,'target_seconds':5}
def command(*args):
 return subprocess.run(args,check=True,capture_output=True,text=True,timeout=25).stdout.strip()
def sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb') as f:
  for b in iter(lambda:f.read(1048576),b''):h.update(b)
 return h.hexdigest()
def save(path,obj):
 raw=(json.dumps(obj,sort_keys=True,indent=2)+'\n').encode();path=Path(path)
 if path.exists():
  if path.is_symlink() or path.read_bytes()!=raw:raise ValueError('immutable record differs: '+str(path))
  return
 tmp=path.with_name(path.name+'.writing')
 with tmp.open('xb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 tmp.chmod(0o600);os.link(tmp,path);tmp.unlink()
def read(path):return json.loads(Path(path).read_text())
def properties(unit):
 return dict(x.split('=',1) for x in command('systemctl','show',unit,'--property=LoadState,ActiveState,SubState,MainPID,Result,ExecMainStatus,User,Group,NoExecPaths,ExecPaths,RootDirectory,RootImage,FragmentPath').splitlines() if '=' in x)
def validate_plan(plan):
 if set(plan)!={'schema','chain_id','activation','binary_sha256','nodes','common_height','commitments'} or plan['schema']!=1 or plan['chain_id']!=4735490 or plan['binary_sha256']!=BIN_SHA:raise ValueError('invalid plan')
 a=dict(plan['activation']);height=a.pop('activation_height',None)
 if a!=PROFILE or type(height)is not int or height<=max(n['height'] for n in plan['nodes'])+1000:raise ValueError('activation profile/margin invalid')
 if {n['node_id'] for n in plan['nodes']}!=set(TARGETS) or len(plan['nodes'])!=5:raise ValueError('five nodes required')
 return hashlib.sha256(json.dumps(plan,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def paths(node):
 if node not in TARGETS:raise ValueError('unknown node')
 unit='hashburst-hvm-testnet'+('-ingress' if node.endswith('ingress') else '')+'.service'
 cfg=Path('/etc')/unit[:-8]/'node.json'
 return unit,cfg
def stopped(unit):
 p=properties(unit)
 if p['ActiveState'] not in ('inactive','failed') or int(p['MainPID'])!=0:raise ValueError('node must be stopped')
def fingerprints(cfg):
 data=Path(cfg['data_dir']);out={}
 for name in ('blockchain.dat','blockchain.idx','consensus-votes.jsonl','consensus-bft-signatures.jsonl','consensus-recovery.json'):
  p=data/name
  if not p.exists():
   if name=='consensus-recovery.json':continue
   raise ValueError('missing state file '+name)
  if p.is_symlink() or not p.is_file():raise ValueError('nonregular state file')
  out[name]={'size':p.stat().st_size,'sha256':sha(p)}
 return out
def check_snapshot(cfg,snapshot):
 if fingerprints(cfg)!=snapshot:raise ValueError('stopped state or journal changed')
def guard_text(directory):
 return '[Service]\nRestart=no\nExecStartPre=/usr/bin/test -f '+str(directory/'START_AUTHORIZED')+'\n'
def start_text(cfg):return '[Service]\nExecStart=\nExecStart='+str(BIN)+' --config '+str(cfg)+'\n'
def local_gate(directory,cfg,unit,plan):
 stopped(unit)
 result=read(directory/'DONE.json')
 if result.get('ok') is not True or result.get('plan')!=validate_plan(plan):raise ValueError('offline job not successful')
 if read(cfg)!=read(directory/'candidate.json'):raise ValueError('candidate not installed')
 check_snapshot(read(cfg),read(directory/'snapshot.json'))
 if sha(BIN)!=BIN_SHA or sha(Path(read(cfg)['data_dir'])/'runtime.pin')!=result['pin_sha256']:raise ValueError('binary/pin changed')
 return result

def worker(directory):
 """Runs as a separate systemd service, independent of SSH lifetime."""
 directory=Path(directory);record=read(directory/'record.json');unit,cfg=paths(record['node_id'])
 try:
  stopped(unit);check_snapshot(read(directory/'old.json'),read(directory/'snapshot.json'))
  if sha(BIN)!=BIN_SHA:raise ValueError('binary changed')
  with (directory/'migration.log').open('ab',buffering=0) as log:
   subprocess.run([str(BIN),'--config',str(cfg),'--migrate-apow',str(directory/'candidate.json')],stdout=log,stderr=log,check=True)
   # Use the runtime account for the full offline check, including file access.
   user=record['service_user']
   subprocess.run(['runuser','-u',user,'--',str(BIN),'--config',str(cfg),'--check'],stdout=log,stderr=log,check=True)
  if read(cfg)!=read(directory/'candidate.json'):raise ValueError('post-migration config mismatch')
  check_snapshot(read(cfg),read(directory/'snapshot.json'))
  save(directory/'DONE.json',{'ok':True,'plan':record['plan'],'pin_sha256':sha(Path(read(cfg)['data_dir'])/'runtime.pin')})
 except Exception as e:
  save(directory/'FAILED.json',{'error':str(e),'plan':record['plan']});raise

def main(p):
 action=p['action'];node=p['node_id'];unit,cfg=paths(node)
 if os.geteuid()!=0:raise ValueError('root required')
 if 'inet '+TARGETS[node]+'/' not in command('ip','-4','addr','show'):raise ValueError('wrong host')
 if action in ('status','commitment'):return inspect(p)
 plan=p['plan'];pid=validate_plan(plan);directory=Path('/var/lib/hashburst-apow-rollout')/pid
 drop=Path('/etc/systemd/system')/(unit+'.d');guard=drop/'80-apow-migration-guard.conf';release=drop/'81-apow-runtime.conf'
 expected=next(n for n in plan['nodes'] if n['node_id']==node)
 if action=='prepare':
  if (directory/'record.json').exists():
   if read(directory/'plan.json')!=plan:raise ValueError('recorded plan differs')
   return {'prepared':True,'retained':True}
  before=inspect({'action':'status','node_id':node});current=read(cfg);props=properties(unit)
  for key in ('digest','peer_id','pin_sha256','binary_sha256','genesis'):
   if before[key]!=expected[key]:raise ValueError('live baseline changed: '+key)
  if current['protocol'].get('apow') or plan['activation']['activation_height']-before['height']<=1500:raise ValueError('activation too close or configured')
  if sha(BIN)!=BIN_SHA or BIN.is_symlink():raise ValueError('candidate binary mismatch')
  # Do not weaken or guess execution allowlists: reject before stopping anything.
  if any(props.get(k) for k in ('NoExecPaths','ExecPaths','RootDirectory','RootImage')):raise ValueError('runtime execution sandbox needs an explicit compatible release path; no changes made')
  user=props.get('User')
  if not user or user=='root':raise ValueError('dedicated runtime account required')
  probe=subprocess.run(['runuser','-u',user,'--',str(BIN),'--help'],capture_output=True,timeout=10)
  if probe.returncode not in (0,2) or b'migrate-apow' not in probe.stderr+probe.stdout:raise ValueError('candidate cannot execute as runtime user')
  miner=properties('hashburst-apow-miner.service')
  if miner.get('ActiveState') in ('active','activating','deactivating'):raise ValueError('miner already running')
  directory.mkdir(parents=True,mode=0o755,exist_ok=True)
  if directory.is_symlink():raise ValueError('symlink rollout directory')
  save(directory/'plan.json',plan);save(directory/'old.json',current)
  next_config=copy.deepcopy(current);next_config['protocol']['apow']=plan['activation'];save(directory/'candidate.json',next_config)
  save(directory/'record.json',{'node_id':node,'plan':pid,'service_user':user})
  save(directory/'unit-before.json',{'text':command('systemctl','cat',unit)})
  return {'prepared':True,'node_id':node,'plan':pid}
 if read(directory/'plan.json')!=plan:raise ValueError('remote plan differs')
 if action=='stop':
  if (directory/'START_AUTHORIZED').exists():raise ValueError('start already authorized; refuse another stop')
  drop.mkdir(mode=0o755,exist_ok=True)
  text=guard_text(directory)
  if guard.exists() and guard.read_text()!=text:raise ValueError('different migration guard')
  guard.write_text(text);guard.chmod(0o644);command('systemctl','daemon-reload')
  command('systemctl','stop','--no-block',unit)
  return {'stop_requested':True}
 if action=='snapshot':
  stopped(unit)
  if (directory/'snapshot.json').exists():
   check_snapshot(read(directory/'old.json'),read(directory/'snapshot.json'));return {'stopped':True,'retained':True}
  if read(cfg)!=read(directory/'old.json'):raise ValueError('config changed before snapshot')
  save(directory/'snapshot.json',fingerprints(read(cfg)))
  return {'stopped':True,'node_id':node}
 if action=='launch':
  stopped(unit)
  if (directory/'DONE.json').exists():return local_gate(directory,cfg,unit,plan)
  if (directory/'FAILED.json').exists():raise ValueError('failed offline job retained; inspect migration.log')
  job='hvm-apow-migrate-'+pid[:16]+'.service'
  state=properties(job)
  if state.get('LoadState')=='loaded':return {'job':state,'retained':True}
  if (directory/'LAUNCHED.json').exists():raise ValueError('job disappeared after launch; no automatic migration retry')
  script=directory/'worker.py'
  # SOURCE contains this module plus inspected shared helpers, never key contents.
  if script.exists() and script.read_text()!=p['worker_source']:raise ValueError('worker source changed')
  script.write_text(p['worker_source']);script.chmod(0o600)
  save(directory/'LAUNCHED.json',{'plan':pid})
  command('systemd-run','--unit='+job,'--property=Type=oneshot','--property=RemainAfterExit=yes','--property=TimeoutStartSec=infinity','--property=Restart=no','/usr/bin/python3',str(script),'--worker',str(directory))
  return {'launched':job}
 if action=='job':
  for name in ('FAILED.json','DONE.json'):
   if (directory/name).exists():return {'file':name,'result':read(directory/name)}
  return {'job':properties('hvm-apow-migrate-'+pid[:16]+'.service')}
 if action=='gate':return local_gate(directory,cfg,unit,plan)
 if action=='start-gate':
  if (directory/'START_AUTHORIZED').exists():
   done=read(directory/'DONE.json')
   if done.get('ok') is not True or done.get('plan')!=pid or read(cfg)!=read(directory/'candidate.json') or sha(BIN)!=BIN_SHA or sha(Path(read(cfg)['data_dir'])/'runtime.pin')!=done['pin_sha256']:raise ValueError('started release differs')
   if release.read_text()!=start_text(cfg):raise ValueError('runtime override changed')
   return {'start_already_authorized':True}
  return local_gate(directory,cfg,unit,plan)
 if action=='install':
  if (directory/'START_AUTHORIZED').exists():return {'already_installed':True}
  local_gate(directory,cfg,unit,plan);text=start_text(cfg)
  if release.exists() and release.read_text()!=text:raise ValueError('runtime override collision')
  release.write_text(text);release.chmod(0o644);command('systemctl','daemon-reload')
  return {'installed':True}
 if action=='start':
  if (directory/'START_AUTHORIZED').exists():return {'start_already_requested':True,'service':properties(unit)}
  local_gate(directory,cfg,unit,plan)
  if release.read_text()!=start_text(cfg):raise ValueError('runtime override missing')
  save(directory/'START_AUTHORIZED',{'plan':pid})
  command('systemctl','start','--no-block',unit)
  return {'started':True}
 if action=='restart-proof':
  if (directory/'RESTART_REQUESTED.json').exists():raise ValueError('restart already requested; use restart-check')
  before=inspect({'action':'status','node_id':node})
  if before['binary_sha256']!=BIN_SHA or before['protocol'].get('apow')!=plan['activation']:raise ValueError('wrong running candidate')
  proof=protected(read(cfg));save(directory/'RESTART_PREFIX.json',proof)
  save(directory/'RESTART_BEFORE.json',before)
  # Intent precedes restart: after a lost SSH reply never repeat this action.
  save(directory/'RESTART_REQUESTED.json',{'plan':pid})
  command('systemctl','restart','--no-block',unit);return {'restart_requested':True}
 if action=='restart-check':
  verify_protected(read(directory/'RESTART_PREFIX.json'));before=read(directory/'RESTART_BEFORE.json');after=inspect({'action':'status','node_id':node})
  if after['height']<=before['height'] or after['peer_id']!=before['peer_id'] or after['binary_sha256']!=BIN_SHA:raise ValueError('restart recovery not ready')
  return {'recovered':True,'node_id':node,'height':after['height']}
 raise ValueError('unknown lifecycle action')
