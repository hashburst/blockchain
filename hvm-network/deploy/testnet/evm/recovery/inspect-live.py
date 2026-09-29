#!/usr/bin/env python3
"""Single-pass, read-only startup diagnostics over new SSH connections."""
import hashlib,json,shlex,subprocess,tempfile,sys
from pathlib import Path
R=Path(__file__).resolve().parent
HOSTS=[('77.90.188.'+ip,'hvm-testnet-v'+str(i),'validator') for i,ip in enumerate(('153','154','155','157'),1)]+[('64.31.4.9','hvm-testnet-ingress','observer')]
REMOTE=r'''
import json,subprocess,urllib.request
from pathlib import Path
def run(args):
 try:
  r=subprocess.run(args,text=True,capture_output=True,timeout=15)
  return {'exit':r.returncode,'stdout':r.stdout,'stderr':r.stderr}
 except Exception as e:return {'error':str(e)}
def ready(h,node,role):
 return h.get('chain_id')==4735490 and h.get('node_id')==node and h.get('role')==role and h.get('reactor_running') is True and h.get('peer_count',0)>=1
prefix='hashburst-hvm-testnet'+('-ingress' if P['role']=='observer' else '')
unit=prefix+'.service'
out={'node_id':P['node_id'],'service':unit,'ready':False}
out['service_status']=run(['systemctl','show',unit,'-p','ActiveState','-p','SubState','-p','MainPID','-p','Result','-p','NRestarts','-p','ExecMainStatus'])
out['processes']=run(['ps','-C','hashburst-testnet','-o','pid,ppid,etime,time,pcpu,rss,stat,wchan'])
out['listener']=run(['ss','-ltnp','sport = :18009'])
try:
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
 with opener.open('http://127.0.0.1:18009/health',timeout=10) as r:h=json.load(r)
 out['health']=h
 out['ready']=ready(h,P['node_id'],P['role'])
except Exception as e:out['health_error']=str(e)
proof=Path('/opt')/prefix/('evm-repair-'+P['sha256'][:12])/'result.json'
try:
 p=json.loads(proof.read_text());out['offline']={k:p.get(k) for k in ('ok','node_id','binary_sha256','evm')}
 out['ready']=out['ready'] and p.get('ok') is True and p.get('binary_sha256')==P['sha256'] and p.get('node_id')==P['node_id']
except Exception as e:out['offline_error']=str(e);out['ready']=False
out['log']=run(['journalctl','-u',unit,'-n','35','--no-pager','-o','short-iso'])
print('HVM_DIAGNOSTIC_JSON:'+json.dumps(out),flush=True)
'''
def main():
 sha=hashlib.sha256((R/'hashburst-testnet').read_bytes()).hexdigest()
 out=Path(tempfile.mkdtemp(prefix='startup-diagnostic-',dir=R));all_ready=True
 for host,node,role in HOSTS:
  print('INSPECT='+host,flush=True)
  payload={'node_id':node,'role':role,'sha256':sha}
  code='import json\nP=json.loads('+repr(json.dumps(payload))+')\n'+REMOTE
  command=['ssh','-T','-o','ControlMaster=no','-o','ControlPath=none','-o','ConnectTimeout=10','-o','ServerAliveInterval=10','-o','ServerAliveCountMax=3','root@'+host,'python3 -']
  try:
   r=subprocess.run(command,input=code,text=True,capture_output=True,timeout=180)
   (out/(host+'.txt')).write_text(r.stdout+'\n'+r.stderr)
   if r.returncode:raise RuntimeError('SSH_EXIT='+str(r.returncode)+' '+r.stderr[-1500:])
   line=next(l for l in reversed(r.stdout.splitlines()) if l.startswith('HVM_DIAGNOSTIC_JSON:'))
   d=json.loads(line.split(':',1)[1]);(out/(host+'.json')).write_text(json.dumps(d,indent=2))
   print(json.dumps(d,indent=2),flush=True);all_ready=all_ready and d['ready']
  except Exception as e:
   all_ready=False;(out/(host+'-error.txt')).write_text(str(e));print('DIAGNOSTIC_FAILED '+host+' '+str(e),flush=True)
 print('LOGS='+str(out),flush=True)
 print('FIVE_RUNTIME_READINESS_OK' if all_ready else 'READINESS_NOT_CONFIRMED_NO_SERVICE_CHANGED',flush=True)
 return 0 if all_ready else 2
if __name__=='__main__':sys.exit(main())
