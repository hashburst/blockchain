#!/usr/bin/env python3
"""Resume the pinned rc.1 TESTNET rollout with a separately versioned installer.
No ledger conversion, forced restart, mainnet activation or GitHub publication.
"""
import argparse, base64, hashlib, importlib.util, json, os, subprocess, sys
from pathlib import Path
R=Path(__file__).resolve().parent
OLD='9dd8e4113c49a90861f465c4b7a18e8db5d7d6aa'
BINARY='3a77bcd4198a666801ca3ea8a91fccd4cead9bae506f6455ebc5ad5a9f8f3ebc'

def revision_file(code):
    encoded=base64.b64encode(code).decode();sha=hashlib.sha256(code).hexdigest()
    return '''
import base64, hashlib, os
from pathlib import Path
revision_code=base64.b64decode(%r)
revision_path=Path('/root/hashburst-installer-revisions')/%r/'installer.py'
revision_path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
if revision_path.parent.resolve()!=revision_path.parent or revision_path.is_symlink():
 raise RuntimeError('installer revision path symlink')
if not revision_path.exists():
 with revision_path.open('xb') as f:
  f.write(revision_code);f.flush();os.fsync(f.fileno())
if hashlib.sha256(revision_path.read_bytes()).hexdigest()!=%r:
 raise RuntimeError('installer revision changed')
''' % (encoded,sha,sha)

INTERCEPT='''
import subprocess
original_run=subprocess.run
def revised_run(args,*a,**kw):
 if isinstance(args,(list,tuple)) and len(args)>2 and Path(str(args[1])).name=='installer.py' and args[2]=='install':
  args=list(args);args[1]=str(revision_path)
 return original_run(args,*a,**kw)
subprocess.run=revised_run
'''
WAIT='''
import json, subprocess, sys, time
root=Path('/var/lib/hashburst-runtime-installer')/p['node']/p['sha256']
state_path=root/'state.json'
job='hvm-runtime-'+p['node']+'-'+p['sha256'][:16]+'.service'
end=time.monotonic()+7200
while time.monotonic()<end:
 record=json.loads(state_path.read_text()) if state_path.exists() else {'phase':'not_started'}
 if record.get('phase')=='verified':
  print('HB_RESULT='+json.dumps(record));break
 status=subprocess.run(['systemctl','show',job,'-p','ActiveState','--value'],check=True,capture_output=True,text=True).stdout.strip()
 if status not in ('active','activating'):
  if record.get('phase')!='start_requested':
   raise RuntimeError('Worker stopped at '+record['phase']+'; no automatic install/restart')
  # Verify the already requested runtime; immutable job artifacts remain intact.
  subprocess.run([sys.executable,str(revision_path),'verify','--node',p['node'],
   '--release',str(root/'release.json'),'--binary',str(root/'candidate'),'--timeout','7200'],check=True)
  record=json.loads(state_path.read_text())
  if record.get('phase')!='verified':raise RuntimeError('missing verified record')
  print('HB_RESULT='+json.dumps(record));break
 time.sleep(15)
else:raise RuntimeError('worker deadline; job retained')
'''

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--deployer-root',type=Path,required=True)
    args=parser.parse_args()
    root=args.deployer_root.resolve(strict=True)
    spec=importlib.util.spec_from_file_location('deployer',root/'hashburst_deployer.py')
    d=importlib.util.module_from_spec(spec);spec.loader.exec_module(d)
    if d.COMMIT!=OLD:raise RuntimeError('unexpected binary source pin')
    s=d.state()
    if s['sha256']!=BINARY:raise RuntimeError('unexpected rollout binary')
    repo=R.parents[2]
    revision=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
    tracked=['hvm-network/deploy/runtime/'+n for n in ('hashburst-install.py','resume-rollout.py','installer-revision.json')]
    for name in tracked:
        expected=subprocess.check_output(['git','show',revision+':'+name],cwd=repo)
        if (repo/name).read_bytes()!=expected:raise RuntimeError('revision file modified: '+name)
    manifest=json.loads((R/'installer-revision.json').read_text())
    code=(R/'hashburst-install.py').read_bytes()
    if manifest['binary_source_commit']!=OLD or manifest['rollout_binary_sha256']!=BINARY or manifest['installer_sha256']!=hashlib.sha256(code).hexdigest():
        raise RuntimeError('installer revision manifest mismatch')
    def checks():
        p=d.gh('/repos/'+d.REPO+'/pulls/27')
        if p['head']['sha']!=revision or p['base']['sha']!=d.BASE or p['merged']:
            raise RuntimeError('PR/base changed; review required')
        for commit in (OLD,revision):
            runs=d.gh('/repos/'+d.REPO+'/commits/'+commit+'/check-runs?per_page=100')
            if runs['total_count']>100 or len(runs['check_runs'])<4 or any(x['status']!='completed' or x['conclusion']!='success' for x in runs['check_runs']):
                raise RuntimeError('CI incomplete/failed for '+commit)
        # Installer-only revision: runtime, workflows and consensus must be unchanged.
        comparison=d.gh('/repos/'+d.REPO+'/compare/'+OLD+'...'+revision)
        allowed=set(manifest['changed_files'])
        files=comparison.get('files',[])
        if not files or len(files)>=300 or any(x['filename'] not in allowed for x in files):
            raise RuntimeError('revision changes outside installer scope')
        return p
    original_remote=d.remote
    preamble=revision_file(code)
    def remote(host,script,payload=None):
        if script==d.REQUEST:script=preamble+INTERCEPT+script
        elif script==d.WAIT:script=preamble+WAIT
        return original_remote(host,script,payload)
    d.checks=checks;d.remote=remote
    sys.argv=['hashburst_deployer.py','deploy']
    # Preserve the original coordinator lock and persistent progress handling.
    d.main()
    receipt={'installer_revision_commit':revision,**manifest}
    d.save(root/'installer-revision-used.json',receipt)
    print('INSTALLER_REVISION_RECORDED_TESTNET_ONLY')

if __name__=='__main__':
    try:main()
    except (Exception,KeyboardInterrupt) as e:
        raise SystemExit('STOP: '+str(e)+'; state and jobs retained; no rollback')
