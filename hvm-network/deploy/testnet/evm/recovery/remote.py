"""Remote guarded repair actions; no keys or signing journal contents exported."""
import base64,hashlib,json,os,subprocess,time
from pathlib import Path
def cmd(*args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT,timeout=30).strip()
def main(p):
 n=p['node'];prefix='hashburst-hvm-testnet'+('-ingress' if n['role']=='observer' else '')
 service=prefix+'.service';cfg=Path('/etc')/prefix/'node.json';c=json.loads(cfg.read_text());data=Path(c['data_dir'])
 if c['node_id']!=n['node_id'] or c['role']!=n['role'] or c['protocol']['chain_id']!=4735490:raise RuntimeError('node identity mismatch')
 root=Path('/opt')/prefix/('evm-repair-'+p['sha256'][:12]);unit='hvm-evm-repair-'+n['node_id']+'-'+p['sha256'][:12]
 if p['action']=='inspect':return {'service_state':cmd('systemctl','show',service,'-p','ActiveState','--value')}
 if p['action']=='prepare':
  state=cmd('systemctl','show',service,'-p','ActiveState','--value')
  if state not in ('inactive','failed'):raise RuntimeError('repair is offline; active service retained; use verify after completed start')
  root.mkdir(parents=True,exist_ok=True,mode=0o700)
  for name,encoded in p['files'].items():
   if name not in ('hashburst-testnet','offline.py'):raise RuntimeError('unexpected release filename')
   raw=base64.b64decode(encoded,validate=True);dst=root/name
   if name=='hashburst-testnet' and hashlib.sha256(raw).hexdigest()!=p['sha256']:raise RuntimeError('binary checksum')
   if dst.exists():
    if dst.read_bytes()!=raw:raise RuntimeError('immutable repair release differs')
   else:
    with dst.open('xb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
    dst.chmod(0o755 if name=='hashburst-testnet' else 0o600)
  job={'node_id':n['node_id'],'role':n['role'],'config':str(cfg),'service':service,'binary':str(root/'hashburst-testnet'),'sha256':p['sha256'],'evm':p['evm']}
  jobpath=root/'job.json'
  if jobpath.exists():
   if json.loads(jobpath.read_text())!=job:raise RuntimeError('different recorded repair job')
  else:
   with jobpath.open('x') as f:json.dump(job,f);f.flush();os.fsync(f.fileno())
   jobpath.chmod(0o600)
  if (root/'result.json').exists():return {'prepared':True,'already_complete':True}
  loaded=subprocess.run(['systemctl','show',unit,'-p','LoadState','--value'],text=True,capture_output=True,timeout=20)
  load=loaded.stdout.strip()
  if load not in ('loaded','not-found'):raise RuntimeError('cannot inspect offline job: '+loaded.stderr)
  if load!='not-found':
   active=cmd('systemctl','show',unit,'-p','ActiveState','--value')
   if active in ('active','activating'):return {'prepared':True,'existing_job':active}
   if not p.get('retry_offline'):raise RuntimeError('previous offline job failed; inspect journalctl -u '+unit+'; retry requires --retry-offline')
   cmd('systemctl','start','--no-block',unit);return {'prepared':True,'retry_requested':True}
  cmd('systemd-run','--no-block','--unit='+unit,'--property=Type=oneshot','--property=RemainAfterExit=yes','--property=TimeoutStartSec=infinity','/usr/bin/python3','-u',str(root/'offline.py'),str(jobpath))
  return {'prepared':True,'unit':unit}
 if p['action']=='poll':
  result=root/'result.json'
  if result.exists():return {'complete':True,'result':json.loads(result.read_text())}
  state=cmd('systemctl','show',unit,'-p','ActiveState','--value');log=cmd('journalctl','-u',unit,'-n','8','--no-pager','-o','cat')
  phase=json.loads((root/'progress.json').read_text()) if (root/'progress.json').exists() else {}
  process=''
  if phase.get('child_pid'):
   result=subprocess.run(['ps','-p',str(int(phase['child_pid'])),'-o','pid,etime,time,pcpu,rss,stat,wchan'],text=True,capture_output=True,timeout=10);process=result.stdout
  return {'complete':False,'state':state,'progress':phase,'process':process,'log':log}
 if p['action']=='start':
  r=json.loads((root/'result.json').read_text())
  if not r['ok'] or r['binary_sha256']!=p['sha256'] or r['evm']!=p['evm']:raise RuntimeError('offline result mismatch')
  expected='ExecStart='+str(root/'hashburst-testnet')+' --config '+str(cfg)
  if expected not in (Path('/etc/systemd/system')/(service+'.d')/'50-evm-release.conf').read_text():raise RuntimeError('service release mismatch')
  subprocess.run(['systemctl','start','--no-block',service],check=True,timeout=20)
  return {'start_requested':n['node_id']}
 if p['action']=='restart-status':return {'requested':(data/'evm-restart-proof.json').exists()}
 if p['action']=='release':
  pid=int(cmd('systemctl','show',service,'-p','MainPID','--value'))
  if pid<=0:raise RuntimeError('service not running')
  exe=Path('/proc')/str(pid)/'exe'
  with exe.open('rb') as f:
   h=hashlib.sha256()
   for b in iter(lambda:f.read(1048576),b''):h.update(b)
  if h.hexdigest()!=p['sha256']:raise RuntimeError('running binary mismatch')
  if c['protocol'].get('evm')!=p['evm']:raise RuntimeError('running configuration mismatch')
  return {'running_binary_verified':True}
 return base_main(p)
