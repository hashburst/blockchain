#!/usr/bin/env python3
"""Align one testnet runtime per invocation; resume never repeats a start request."""
import argparse, fcntl, json, os, subprocess, sys, time
from pathlib import Path
import importlib.util
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('recovery',R/'recover-v1.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
ORDER=[('64.31.4.9','hvm-testnet-ingress'),('77.90.188.157','hvm-testnet-v4'),('77.90.188.154','hvm-testnet-v2'),('77.90.188.155','hvm-testnet-v3')]
ALL=[(r.HOST,r.NODE)]+r.PEERS
OLD='3a77bcd4198a666801ca3ea8a91fccd4cead9bae506f6455ebc5ad5a9f8f3ebc'
STATE=r'''
from pathlib import Path
import subprocess
root=Path('/var/lib/hashburst-runtime-installer')/p['node']/p['sha']
job='hvm-runtime-'+p['node']+'-'+p['sha'][:16]+'.service'
phase=json.loads((root/'state.json').read_text()).get('phase') if (root/'state.json').exists() else None
active=subprocess.run(['systemctl','show',job,'--property=ActiveState','--value'],check=True,capture_output=True,text=True).stdout.strip()
print('HB_RESULT='+json.dumps({'exists':root.exists(),'phase':phase,'active':active}))
'''
REQUEST=r'''
from pathlib import Path
import hashlib, subprocess, sys
root=Path(p['root'])
if root.resolve()!=root:raise RuntimeError('staging symlink')
for name,expected in p['files'].items():
 f=root/name
 if f.is_symlink() or hashlib.sha256(f.read_bytes()).hexdigest()!=expected:raise RuntimeError('artifact differs')
subprocess.run([sys.executable,str(root/'installer.py'),'install','--node',p['node'],'--release',str(root/'release.json'),'--binary',str(root/'hashburst-testnet'),'--timeout','21600'],check=True)
print('HB_RESULT={"requested":true}')
'''
VERIFY=r'''
from pathlib import Path
import subprocess,sys
root=Path('/var/lib/hashburst-runtime-installer')/p['node']/p['sha']
subprocess.run([sys.executable,str(root/'installer.py'),'verify','--node',p['node'],'--release',str(root/'release.json'),'--binary',str(root/'candidate'),'--timeout','180'],check=True)
print('HB_RESULT={"verified":true}')
'''

def check_binaries(rows, completed):
 for row in rows:
  expected=r.BINARY_SHA if row['node_id'] in set(completed)|{r.NODE} else OLD
  r.require(row['binary_sha256']==expected,'unexpected runtime: '+row['node_id'])

def wait(d,host,node,timeout):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  s=d.remote(host,STATE,{'node':node,'sha':r.BINARY_SHA})
  print('FLEET_PHASE='+str(s['phase'])+' JOB='+s['active'],flush=True)
  if s['active'] in ('active','activating'):
   time.sleep(30);continue
  r.require(s['phase'] in ('start_requested','verified'),'worker stopped before start; inspect journal; no automatic restart')
  d.remote(host,VERIFY,{'node':node,'sha':r.BINARY_SHA});return
 raise RuntimeError('monitor deadline; rerun same command; persistent job retained')

def main():
 a=argparse.ArgumentParser(description=__doc__)
 a.add_argument('--deployer-root',type=Path,required=True)
 a.add_argument('--timeout',type=int,default=21600)
 args=a.parse_args();r.require(0<args.timeout<=86400,'invalid timeout')
 root=args.deployer_root.resolve(strict=True);os.umask(0o077)
 d=r.load_module('deployer',root/'hashburst_deployer.py')
 r.require(d.ROOT.resolve()==root,'deployer root differs')
 with (root/'deployer.lock').open('a') as lock:
  fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
  repo=R.parents[2]
  for name in ('align-fleet.py','fleet-install.py','fleet-release.json','recover-v1.py','../testnet/apow/preflight.py'):
   p=(R/name).resolve();rel=p.relative_to(repo)
   r.require(subprocess.check_output(['git','-C',str(repo),'show','HEAD:'+str(rel)])==p.read_bytes(),'modified rollout source: '+name)
  rev=subprocess.check_output(['git','-C',str(repo),'rev-parse','HEAD'],text=True).strip()
  checks=d.gh('/repos/'+r.REPO+'/commits/'+rev+'/check-runs?per_page=100')
  r.require(checks and checks['total_count']<=100 and len(checks['check_runs'])>=3 and all(x['status']=='completed' and x['conclusion']=='success' for x in checks['check_runs']),'revision CI not successful')
  release=json.loads((R/'fleet-release.json').read_text())
  r.require(release['source_commit']==r.SOURCE and release['sha256']==r.BINARY_SHA and release['predecessors']==[OLD] and 'failed_node_recovery' not in release,'manifest differs')
  r.require(r.sha(R/'fleet-install.py')==release['installer_sha256'],'installer digest differs')
  cached=root/('v1-recovery-'+r.SOURCE[:12]);accept=json.loads((cached/'ACCEPTANCE.json').read_text())
  r.require(accept['sha256']==r.BINARY_SHA and accept['source_commit']==r.SOURCE,'v1 acceptance differs')
  r.require(r.sha(cached/'hashburst-testnet')==r.BINARY_SHA,'candidate digest differs')
  out=root/('fleet-alignment-'+r.SOURCE[:12]);out.mkdir(exist_ok=True)
  state=out/'state.json'
  s=json.loads(state.read_text()) if state.exists() else {'revision':rev,'completed':[],'in_flight':None}
  r.require(s['revision']==rev,'rollout revision differs; explicit review required')
  r.require(s['completed']==[n for _,n in ORDER][:len(s['completed'])],'invalid completed order')
  m=r.load_module('preflight',R.parent/'testnet/apow/preflight.py')
  target=next(((h,n) for h,n in ORDER if n not in s['completed']),None)
  if s['in_flight']:
   r.require(target is not None and s['in_flight']==list(target),'in-flight target differs')
   wait(d,*target,args.timeout)
  elif target:
   before=r.gate(d,m,ALL,out/('before-'+str(time.time_ns())+'.json'))
   check_binaries(before['nodes'],s['completed'])
   host,node=target
   remote=d.remote(host,STATE,{'node':node,'sha':r.BINARY_SHA})
   r.require(not remote['exists'],'untracked candidate job exists; inspect rather than overwrite')
   stage='/root/hashburst-fleet-alignment-'+r.BINARY_SHA
   d.remote(host,"from pathlib import Path\nx=Path(p['root']);x.mkdir(mode=0o700,exist_ok=True)\nif x.resolve()!=x:raise RuntimeError('staging symlink')\nprint('HB_RESULT={}')",{'root':stage})
   for name,source in [('installer.py',R/'fleet-install.py'),('release.json',R/'fleet-release.json'),('hashburst-testnet',cached/'hashburst-testnet')]:
    dest=out/name
    if dest.exists():r.require(r.sha(dest)==r.sha(source),'local artifact differs')
    else:dest.write_bytes(source.read_bytes())
   files={n:r.sha(out/n) for n in ('installer.py','release.json','hashburst-testnet')}
   subprocess.run(['scp',*d.SSH_OPTIONS,*[str(out/n) for n in files],'root@'+host+':'+stage+'/'],check=True)
   s['in_flight']=list(target);s['before']=before;d.save(state,s)
   d.remote(host,REQUEST,{'root':stage,'node':node,'files':files})
   wait(d,host,node,args.timeout)
  after=r.gate(d,m,ALL,out/('after-'+str(time.time_ns())+'.json'))
  completed=s['completed']+([target[1]] if target else [])
  check_binaries(after['nodes'],completed)
  if target:
   old={x['node_id']:x for x in s['before']['nodes']}
   for row in after['nodes']:
    r.require(all(row[k]==old[row['node_id']][k] for k in ('digest','genesis','peer_id','pin_sha256')),'identity/state pin changed')
   s.update(completed=completed,in_flight=None);d.save(state,s)
   print('ONE_RUNTIME_ALIGNED='+target[1],flush=True)
  if len(completed)==len(ORDER):
   d.save(out/'ACCEPTANCE.json',{'revision':rev,'source_commit':r.SOURCE,'sha256':r.BINARY_SHA,'gate':after,'mainnet_activated':False})
   print('FIVE_CORRECTED_RUNTIMES_AGREEMENT_AND_PROGRESS_OK')
  else:print('NEXT: repeat the same command for the next node')
  print('NO_MINER_CONFIGURATION_OR_MAINNET_CHANGE; DO_NOT_RUN_OLD_DEPLOY_OR_PUBLISH')

if __name__=='__main__':
 try:main()
 except (Exception,KeyboardInterrupt) as e:
  print('STOP: '+str(e)+'; state retained; no automatic rollback',file=sys.stderr);sys.exit(1)
