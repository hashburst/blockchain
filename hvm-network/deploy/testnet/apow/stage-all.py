#!/usr/bin/env python3
"""Five-node read-only gate then immutable staging. No migration or restart."""
import argparse,concurrent.futures,hashlib,importlib.util,json,subprocess,tarfile,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('preflight',R/'preflight.py');pre=importlib.util.module_from_spec(spec);spec.loader.exec_module(pre)
ARCHIVE_SHA256='70413dc9345b1d20b0255a6770c0972f8fe914a6592be4c20c4dddd13fe20c37'
ROOT='HashBurst-HVM-v0.4.0-rc.1-linux-amd64'
def unpack_release(archive,out):
 if hashlib.sha256(Path(archive).read_bytes()).hexdigest()!=ARCHIVE_SHA256:raise ValueError('expected exact v0.4.0-rc.1 Linux amd64 release archive')
 with tarfile.open(archive,'r:gz') as tar:
  members={}
  for m in tar.getmembers():
   if m.name in (ROOT+'/hashburst-testnet',ROOT+'/hvm-apow-miner'):
    if m.name in members or not m.isfile() or m.size>200_000_000:raise ValueError('invalid or duplicate archive member')
    members[m.name]=m
  if len(members)!=2:raise ValueError('release binaries missing')
  for name,m in members.items():
   (out/Path(name).name).write_bytes(tar.extractfile(m).read())
   (out/Path(name).name).chmod(0o755)
def main():
 p=argparse.ArgumentParser();p.add_argument('--release',required=True);a=p.parse_args()
 out=Path(tempfile.mkdtemp(prefix='apow-stage-results-',dir=R));sessions=[]
 try:
  with tempfile.TemporaryDirectory(prefix='hvm-apow-upload-') as t:
   upload=Path(t);unpack_release(a.release,upload)
   (upload/'miner-service.py').write_bytes((R/'miner-service.py').read_bytes())
   (upload/'miner-release.json').write_text(json.dumps({'sha256':hashlib.sha256((upload/'hvm-apow-miner').read_bytes()).hexdigest()}))
   manifest={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in upload.iterdir()}
   (upload/'stage-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
   source=pre.SOURCE+'\ninspect=main\n'+(R/'stage-node.py').read_text()
   for host,node in pre.TARGETS:
    print('SSH_AUTH='+host,flush=True);sessions.append(pre.transport.Session(pre.transport.ssh_command(host),source))
   def rows():
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
     return list(pool.map(lambda x:x[0].call({'action':'status','node_id':x[1][1],'_timeout':45}),zip(sessions,pre.TARGETS)))
   first=rows();pre.validate_rows(first)
   if any(x['protocol'].get('apow') for x in first):raise ValueError('APoW already configured; staging workflow not applicable')
   height=min(x['height'] for x in first)
   proofs=[s.call({'action':'commitment','height':height,'_timeout':45}) for s in sessions];pre.compare(proofs,height)
   (out/'nodes.json').write_text(json.dumps(first,indent=2));(out/'commitments.json').write_text(json.dumps(proofs,indent=2))
   time.sleep(5);later=rows();pre.validate_rows(later)
   for x,y in zip(first,later):
    if y['height']<=x['height'] or y['digest']!=x['digest'] or y['pin_sha256']!=x['pin_sha256']:raise ValueError('finality/configuration gate failed')
   (out/'progress.json').write_text(json.dumps(later,indent=2))
   print('FIVE_NODE_AGREEMENT_AND_PROGRESS_OK height='+str(height),flush=True)
   for s,(host,node) in zip(sessions,pre.TARGETS):
    remote=s.call({'action':'prepare-upload'})
    if not remote.startswith('/root/hvm-apow-stage-') or any(c not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/-_' for c in remote):raise ValueError('invalid remote upload directory')
    print('UPLOAD_AND_STAGE='+node,flush=True)
    subprocess.run(['scp','-o','ControlMaster=no','-o','ControlPath=none',*[str(x) for x in sorted(upload.iterdir())],'root@'+host+':'+remote+'/'],check=True,timeout=600)
    result=s.call({'action':'stage','node_id':node,'upload':remote,'manifest':manifest,'_timeout':180})
    (out/(node+'.json')).write_text(json.dumps(result,indent=2)+'\n');print('APOW_CANDIDATE_STAGED='+node,flush=True)
   final=rows();pre.validate_rows(final)
   if any(a['pin_sha256']!=b['pin_sha256'] or a['binary_sha256']!=b['binary_sha256'] for a,b in zip(first,final)):raise ValueError('running configuration/release changed')
   (out/'final-status.json').write_text(json.dumps(final,indent=2))
   (out/'STAGED.json').write_text(json.dumps({'ok':True,'chain_id':4735490,'node_count':5,'activation_complete':False,'service_restarted':False,'manifest':manifest},indent=2))
   print('FIVE_CANDIDATES_FOUR_MINERS_STAGED_NO_ACTIVATION',flush=True)
 except Exception as e:
  (out/'ERROR.txt').write_text(str(e));raise SystemExit('STOP: '+str(e)+'; staging retained; no node restart or automatic rollback')
 finally:
  for s in sessions:s.close()
  archive=out.with_suffix('.tar.gz')
  with tarfile.open(archive,'w:gz') as tar:tar.add(out,arcname=out.name)
  print('REPORT_ARCHIVE='+str(archive),flush=True)
if __name__=='__main__':main()
