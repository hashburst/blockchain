"""Install a tested testnet runtime without migrating data or signing journals."""
import argparse, hashlib, json, os, re, shutil, subprocess, time, urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
DIAG_UNIT=None
OLD='0e597329261f9376d9c33b1fe427638fd2810bfd43ce5bd702451e785d9f339d'
NODES=['hvm-testnet-v'+str(i) for i in range(1,5)]+['hvm-testnet-ingress']
def require(ok,message):
 if not ok:raise RuntimeError(message)
def run(*args):return subprocess.run(args,check=True,capture_output=True,text=True,timeout=20).stdout.strip()
def digest(path,limit=None):
 h=hashlib.sha256()
 with Path(path).open('rb') as f:
  remaining=limit
  while remaining is None or remaining:
   b=f.read(1048576 if remaining is None else min(remaining,1048576))
   if not b:
    require(remaining in (None,0),'journal truncated');break
   h.update(b)
   if remaining is not None:remaining-=len(b)
 return h.hexdigest()
def props(unit):return dict(x.split('=',1) for x in run('systemctl','show',unit,'--property=ActiveState,MainPID,User,RootDirectory,RootImage,ExecStart').splitlines() if '=' in x)
def effective(raw,binary,cfg):
 return raw.count('path=')==1 and 'path='+str(binary)+' ;' in raw and 'argv[]='+str(binary)+' --config '+str(cfg)+' ;' in raw
def atomic(path,value):
 temp=path.with_suffix('.tmp')
 with temp.open('x') as f:json.dump(value,f,indent=2);f.flush();os.fsync(f.fileno())
 temp.chmod(0o600);os.replace(temp,path)
def prefix_proof(data):
 proof={}
 for name in ['consensus-votes.jsonl','consensus-bft-signatures.jsonl']:
  p=data/name;require(p.is_file() and not p.is_symlink(),'missing regular journal')
  size=p.stat().st_size;proof[name]={'size':size,'sha256':digest(p,size)}
 return proof
def check_preserved(record,cfg,data):
 require(digest(cfg)==record['config_sha256'],'configuration changed')
 require(digest(data/'runtime.pin')==record['pin_sha256'],'identity pin changed')
 for name,p in record['journals'].items():require(digest(data/name,p['size'])==p['sha256'],'journal prefix changed: '+name)
def main():
 global DIAG_UNIT
 a=argparse.ArgumentParser();a.add_argument('action',choices=['install','verify']);a.add_argument('--node',required=True,choices=NODES);a.add_argument('--timeout',type=int,default=7200);args=a.parse_args()
 require(os.geteuid()==0,'run on the target VPS as root')
 release=json.loads((R/'runtime-release.json').read_text());sha=release['sha256']
 require(re.fullmatch('[0-9a-f]{64}',sha) and release['chain_id']==4735490,'invalid release manifest')
 require(digest(R/'hashburst-testnet')==sha,'candidate checksum mismatch')
 observer=args.node.endswith('ingress');stem='hashburst-hvm-testnet'+('-ingress' if observer else '')
 unit=stem+'.service';DIAG_UNIT=unit;cfg=Path('/etc')/stem/'node.json';c=json.loads(cfg.read_text());data=Path(c['data_dir'])
 require(c['node_id']==args.node and c['network']=='testnet' and c['protocol']['chain_id']==4735490,'node/network mismatch')
 require(c['role']==('observer' if observer else 'validator'),'role mismatch')
 require(c['protocol'].get('apow') and (not observer or not c.get('consensus_key_file')),'APoW migration or non-signing observer guard failed')
 state=Path('/var/lib/hashburst-runtime-repair')/sha
 state.mkdir(parents=True,mode=0o700,exist_ok=True)
 require(not state.is_symlink(),'record directory is a symlink')
 record_path=state/'record.json';binary=Path('/opt/hashburst-runtime-repair')/sha/'hashburst-testnet'
 drop=Path('/etc/systemd/system')/(unit+'.d')/'zz-hvm-lock-recovery.conf'
 p=props(unit);require(not p.get('RootDirectory') and not p.get('RootImage'),'mapped service root requires review')
 if args.action=='install':
  if record_path.exists():
   print('INSTALL_ALREADY_RECORDED_USE_VERIFY');return
  require(not drop.exists(),'existing recovery override requires review')
  raw=p['ExecStart'];match=re.search(r'path=([^ ;]+)',raw)
  require(match is not None and digest(match.group(1))==OLD,'unexpected installed runtime; retained')
  pid=int(p['MainPID'])
  if pid:require(digest('/proc/'+str(pid)+'/exe')==OLD,'unexpected running runtime; retained')
  binary.parent.mkdir(parents=True,mode=0o755,exist_ok=True)
  require(not binary.parent.is_symlink(),'release directory is a symlink')
  if not binary.exists():shutil.copyfile(R/'hashburst-testnet',binary)
  require(not binary.is_symlink() and digest(binary)==sha,'installed candidate differs')
  binary.chmod(0o755)
  run('runuser','-u',p['User'],'--','test','-x',str(binary))
  run('systemctl','stop','--no-block',unit)
  deadline=time.monotonic()+90
  while True:
   p=props(unit)
   if int(p['MainPID'])==0 and p['ActiveState'] in ('inactive','failed'):break
   require(time.monotonic()<deadline,'stop pending; no override installed');time.sleep(2)
  record={'node_id':args.node,'config_sha256':digest(cfg),'pin_sha256':digest(data/'runtime.pin'),'journals':prefix_proof(data),'sha256':sha}
  atomic(record_path,record)
  shutil.copytree(drop.parent,state/'dropins-before',dirs_exist_ok=False)
  text='[Service]\nExecStart=\nExecStart='+str(binary)+' --config '+str(cfg)+'\nExecPaths='+str(binary)+'\n'
  with drop.open('x') as f:f.write(text)
  drop.chmod(0o644);run('systemctl','daemon-reload')
  require(effective(props(unit)['ExecStart'],binary,cfg),'effective command differs; service retained stopped')
  check_preserved(record,cfg,data)
  run('systemctl','start','--no-block',unit)
  print('CANDIDATE_START_REQUESTED_NO_MIGRATION_NO_MINER_STARTED');print('NEXT: verify --node '+args.node);return
 require(record_path.exists(),'no installation proof')
 record=json.loads(record_path.read_text());check_preserved(record,cfg,data)
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}));deadline=time.monotonic()+args.timeout;first=None
 while True:
  p=props(unit)
  require(p['ActiveState']!='failed','runtime failed: inspect journal; no automatic restart')
  require(effective(p['ExecStart'],binary,cfg),'effective command changed')
  try:
   pid=int(p['MainPID']);require(pid>0,'no process')
   require(digest('/proc/'+str(pid)+'/exe')==sha,'running binary differs')
   with opener.open('http://127.0.0.1:18009/health',timeout=5) as response:h=json.load(response)
   require(h['chain_id']==4735490 and h['node_id']==args.node and h['peer_id']==c['peer_id'] and h['role']==c['role'],'health identity mismatch')
   if not h.get('reactor_running') or h.get('peer_count',0)<1:
    print('WAIT reactor/peers not ready',flush=True)
    require(time.monotonic()<deadline,'readiness deadline; repeat verify only');time.sleep(15);continue
   if first is None:first=h['finalized_height']
   barrier=c['protocol']['apow']['activation_height']-1
   if h['finalized_height']>first or h['finalized_height']==barrier:
    check_preserved(record,cfg,data)
    if not (state/'verified.json').exists():atomic(state/'verified.json',h)
    print('RUNTIME_RECOVERY_AND_JOURNAL_PREFIX_OK');print('FINALIZED_HEIGHT='+str(h['finalized_height']));return
   print('WAIT finality='+str(h['finalized_height']),flush=True)
  except (OSError,ValueError) as e:print('REPLAY_WAIT '+str(e),flush=True)
  require(time.monotonic()<deadline,'readiness deadline; repeat verify only');time.sleep(15)
if __name__=='__main__':
 try:main()
 except Exception as e:
  if DIAG_UNIT:
   try:
    print(run('systemctl','show',DIAG_UNIT,'--property=ActiveState,SubState,MainPID,Result,ExecMainStatus'),flush=True)
    print(run('journalctl','-u',DIAG_UNIT,'-n','40','--no-pager','-o','short-iso'),flush=True)
   except Exception as diagnostic_error:print('DIAGNOSTIC_UNAVAILABLE: '+str(diagnostic_error),flush=True)
  raise SystemExit('STOP: '+str(e)+'; data and journals retained; no automatic rollback')
