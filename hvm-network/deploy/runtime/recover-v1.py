#!/usr/bin/env python3
"""Pinned failed-v1 recovery. No modification of the original deployment.json.
Run on the administrator's Mac. SSH credentials stay interactive.
"""
import argparse, base64, fcntl, hashlib, importlib.util, json, os
import subprocess, sys, time, urllib.request, zipfile
from pathlib import Path
R = Path(__file__).resolve().parent
REPO = 'hashburst/blockchain'
SOURCE = '2719806647f25012a8d6b868a645c0341b0442c1'
RUN = 37123137869
ARTIFACT = 11274157535
ZIP_SHA = '91a763f4cb4f500a42f03c6a3dae093d963b4a01d3f4c2260fd13a5138279f32'
BINARY_SHA = '30a0cd390af000019577f06bcf97ce54959808aa4447ab318ef669cf8aa9be0e'
OLD_SHA = '10cb427894e3f51c088be2eb06744706f9f080aa9a2877cda6944622ebb8e0b7'
NODE = 'hvm-testnet-v1'
HOST = '77.90.188.153'
PEERS = [('77.90.188.154','hvm-testnet-v2'), ('77.90.188.155','hvm-testnet-v3'),
         ('77.90.188.157','hvm-testnet-v4'), ('64.31.4.9','hvm-testnet-ingress')]
REMOTE_DIR = '/root/hashburst-v1-recovery-' + BINARY_SHA

def require(ok, why):
    if not ok: raise RuntimeError(why)

def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda:f.read(1048576), b''): h.update(b)
    return h.hexdigest()

def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
    return m

class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        from urllib.parse import urlsplit
        require(urlsplit(newurl).scheme == 'https', 'non-HTTPS artifact redirect')
        new = super().redirect_request(req, fp, code, msg, headers, newurl)
        if new is not None:
            new.remove_header('Authorization')
        return new

def compare_rows(rows, targets):
    require({r['node_id'] for r in rows} == {n for _,n in targets} and len(rows)==len(targets), 'node set differs')
    require(len({r['peer_id'] for r in rows})==len(rows), 'duplicate identity')
    for key in ('digest','genesis'):
        require(len({r[key] for r in rows})==1, 'network '+key+' differs')
    require(len({json.dumps(r['protocol'],sort_keys=True) for r in rows})==1, 'protocol differs')
    for r in rows:
        require('ActiveState=active' in r['service'] and 'SubState=running' in r['service'], 'peer not running')

def compare_proofs(proofs, height, fields):
    require(len(proofs) >= 4, 'at least three validators and observer required')
    normalized=[]
    for original in proofs:
        p=dict(original); p.setdefault('evm_gas_used',0)
        require(p.get('chain_id')==4735490 and p.get('height')==height and p.get('certificate'), 'finality evidence missing')
        require(type(p['evm_gas_used']) is int and 0<=p['evm_gas_used']<2**64, 'gas field invalid')
        for k in fields:
            require(p.get(k) is not None and (k in ('height','evm_gas_used','chain_id') or p[k]), 'commitment missing '+k)
        normalized.append(p)
    require(all(all(p[k]==normalized[0][k] for k in fields) for p in normalized), 'finalized commitments differ')

def gate(d, m, targets, output):
    def call(host, payload):
        return d.remote(host, m.SOURCE+'\nprint("HB_RESULT="+json.dumps(main(p)))\n',payload)
    rows=[call(h,{'action':'status','node_id':n}) for h,n in targets]
    compare_rows(rows,targets)
    height=min(r['height'] for r in rows)
    proofs=[call(h,{'action':'commitment','height':height}) for h,n in targets]
    compare_proofs(proofs,height,m.FIELDS)
    time.sleep(5)
    later=[call(h,{'action':'status','node_id':n}) for h,n in targets]
    compare_rows(later,targets)
    for a,b in zip(rows,later):
        require(b['height']>a['height'], 'finality not progressing: '+a['node_id'])
        require(all(a[k]==b[k] for k in ('digest','pin_sha256','peer_id','binary_sha256')), 'runtime or identity changed during gate')
    result={'nodes':later,'height':height,'commitments':proofs,'time':time.time(),
            'qc_signatures_independently_verified':False}
    d.save(output,result)
    print('AGREEMENT_AND_PROGRESS_OK nodes='+str(len(targets))+' height='+str(height),flush=True)
    return result

INSPECT = r'''
import hashlib,subprocess
from pathlib import Path
unit='hashburst-hvm-testnet.service'
out=subprocess.run(['systemctl','show',unit,'--property=ActiveState,SubState,MainPID,ExecMainStatus,Result,ExecStart'],check=True,capture_output=True,text=True).stdout
c=json.loads(Path('/etc/hashburst-hvm-testnet/node.json').read_text())
pin=(Path(c['data_dir'])/'runtime.pin').read_text().splitlines()
if c['node_id']!='hvm-testnet-v1' or c['network']!='testnet' or c['protocol']['chain_id']!=4735490 or c['role']!='validator':raise RuntimeError('target identity differs')
if len(pin)!=4 or pin[1:]!=[c['node_id'],c['peer_id'],c.get('validator_id','')]:raise RuntimeError('target pin differs')
print('HB_RESULT='+json.dumps({'service':out,'digest':pin[0],'protocol':c['protocol'],'genesis':c['genesis_hash'],'peer_id':c['peer_id']}))
'''

REQUEST = r'''
import hashlib,subprocess,sys
from pathlib import Path
root=Path(p['root'])
if root.resolve()!=root:raise RuntimeError('staging symlink')
for name,expected in p['files'].items():
 f=root/name
 if f.is_symlink() or hashlib.sha256(f.read_bytes()).hexdigest()!=expected:raise RuntimeError('artifact differs: '+name)
miner=subprocess.run(['systemctl','show','hashburst-apow-miner.service','--property=ActiveState','--value'],check=True,capture_output=True,text=True).stdout.strip()
if miner not in ('inactive','failed',''):raise RuntimeError('target miner must remain stopped')
subprocess.run([sys.executable,str(root/'installer.py'),'install','--node','hvm-testnet-v1','--release',str(root/'release.json'),'--binary',str(root/'hashburst-testnet'),'--timeout','7200'],check=True)
print('HB_RESULT='+json.dumps({'requested':True}))
'''

WAIT = r'''
import subprocess,time,sys
from pathlib import Path
root=Path('/var/lib/hashburst-runtime-installer/hvm-testnet-v1')/p['sha256']
job='hvm-runtime-hvm-testnet-v1-'+p['sha256'][:16]+'.service'
end=time.monotonic()+7200
while time.monotonic()<end:
 record=json.loads((root/'state.json').read_text()) if (root/'state.json').exists() else {}
 active=subprocess.run(['systemctl','show',job,'--property=ActiveState','--value'],check=True,capture_output=True,text=True).stdout.strip()
 if active in ('active','activating'):
  print('RECOVERY_PHASE='+record.get('phase','starting'),flush=True);time.sleep(15);continue
 if record.get('phase') in ('start_requested','verified'):
  subprocess.run([sys.executable,str(root/'installer.py'),'verify','--node','hvm-testnet-v1','--release',str(root/'release.json'),'--binary',str(root/'candidate'),'--timeout','7200'],check=True)
  print('HB_RESULT='+(root/'state.json').read_text());break
 subprocess.run(['journalctl','-u',job,'-n','40','--no-pager'],check=False,stdout=sys.stderr)
 raise RuntimeError('worker not active at '+record.get('phase','not_started')+'; explicit inspection required, no restart')
else:raise RuntimeError('deadline; persistent job retained, rerun recover-v1.py')
'''

EXISTING = r'''
import subprocess
from pathlib import Path
root=Path('/var/lib/hashburst-runtime-installer/hvm-testnet-v1')/p['sha256']
active=subprocess.run(['systemctl','show','hvm-runtime-hvm-testnet-v1-'+p['sha256'][:16]+'.service','--property=ActiveState','--value'],check=True,capture_output=True,text=True).stdout.strip()
print('HB_RESULT='+json.dumps({'exists':root.exists(),'active':active}))
'''

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--deployer-root',type=Path,required=True)
    a=parser.parse_args(); root=a.deployer_root.resolve(strict=True);os.umask(0o077)
    d=load_module('existing_deployer',root/'hashburst_deployer.py')
    require(d.ROOT.resolve()==root, 'deployer root differs')
    # Lock the same coordinator lock; never rewrite its release or completion state.
    with (root/'deployer.lock').open('a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        repo=R.parents[2]
        tracked=['hvm-network/deploy/runtime/'+n for n in ('recover-v1.py','hashburst-install.py','v1-recovery-release.json')]
        for path in tracked:
            raw=subprocess.check_output(['git','-C',str(repo),'show','HEAD:'+path])
            require(raw==(repo/path).read_bytes(),'modified recovery source: '+path)
        revision=subprocess.check_output(['git','-C',str(repo),'rev-parse','HEAD'],text=True).strip()
        ci=d.gh('/repos/'+REPO+'/commits/'+revision+'/check-runs?per_page=100')
        require(ci and ci['total_count']<=100 and len(ci['check_runs'])>=3 and all(x['status']=='completed' and x['conclusion']=='success' for x in ci['check_runs']), 'recovery revision CI incomplete')
        build=d.gh('/repos/'+REPO+'/actions/runs/'+str(RUN))
        require(build['head_sha']==SOURCE and build['conclusion']=='success', 'candidate CI differs')
        artifact=d.gh('/repos/'+REPO+'/actions/artifacts/'+str(ARTIFACT))
        require(not artifact['expired'] and artifact['digest']=='sha256:'+ZIP_SHA and artifact['workflow_run']['head_sha']==SOURCE, 'candidate artifact differs')
        release=json.loads((R/'v1-recovery-release.json').read_text())
        require(release['source_commit']==SOURCE and release['sha256']==BINARY_SHA and release['predecessors']==[OLD_SHA], 'manifest differs')
        out=root/('v1-recovery-'+SOURCE[:12]);out.mkdir(exist_ok=True)
        archive=out/'candidate.zip'
        if not archive.exists():
            request=urllib.request.Request('https://api.github.com/repos/'+REPO+'/actions/artifacts/'+str(ARTIFACT)+'/zip',headers={'Authorization':'Bearer '+d.TOKEN,'User-Agent':'HashBurst-Recovery'})
            temp=out/'candidate.download'
            with urllib.request.build_opener(SafeRedirect()).open(request,timeout=120) as response,temp.open('wb') as f:
                import shutil
                shutil.copyfileobj(response,f)
            require(sha(temp)==ZIP_SHA, 'archive checksum differs');os.replace(temp,archive)
        require(sha(archive)==ZIP_SHA,'archive checksum differs')
        binary=out/'hashburst-testnet'
        if not binary.exists():
            with zipfile.ZipFile(archive) as z:
                require(z.namelist()==['hashburst-testnet-reactor-candidate'],'unexpected archive contents')
                data=z.read(z.namelist()[0]);require(hashlib.sha256(data).hexdigest()==BINARY_SHA,'binary checksum differs')
                binary.write_bytes(data)
        require(sha(binary)==BINARY_SHA,'local binary differs')
        for name,source in [('installer.py',R/'hashburst-install.py'),('release.json',R/'v1-recovery-release.json')]:
            dest=out/name
            if dest.exists():require(dest.read_bytes()==source.read_bytes(),'immutable local recovery input differs')
            else:dest.write_bytes(source.read_bytes())
        require(sha(out/'installer.py')==release['installer_sha256'],'installer checksum differs')
        m=load_module('recovery_preflight',R.parent/'testnet/apow/preflight.py')
        existing=d.remote(HOST,EXISTING,{'sha256':BINARY_SHA})
        if not existing['exists']:
            before=gate(d,m,PEERS,out/('before-'+str(time.time_ns())+'.json'))
            target=d.remote(HOST,INSPECT)
            peer=before['nodes'][0]
            require(all(target[k]==peer[k] for k in ('digest','protocol','genesis')), 'target network differs')
            require(target['peer_id'] not in {r['peer_id'] for r in before['nodes']}, 'duplicate target identity')
            require('ActiveState=failed' in target['service'] and 'MainPID=0\n' in target['service']+'\n', 'target must be failed and stopped')
            # No shared archive extraction and no overwrite of the original rollout.
            d.remote(HOST,"from pathlib import Path\nr=Path(p['root']);r.mkdir(mode=0o700,exist_ok=True)\nif r.resolve()!=r:raise RuntimeError('staging symlink')\nprint('HB_RESULT={}')",{'root':REMOTE_DIR})
            files={n:sha(out/n) for n in ('installer.py','release.json','hashburst-testnet')}
            subprocess.run(['scp',*d.SSH_OPTIONS,*[str(out/n) for n in files],'root@'+HOST+':'+REMOTE_DIR+'/'],check=True)
            d.remote(HOST,REQUEST,{'root':REMOTE_DIR,'files':files})
        d.remote(HOST,WAIT,{'sha256':BINARY_SHA})
        after=gate(d,m,[(HOST,NODE)]+PEERS,out/('after-'+str(time.time_ns())+'.json'))
        target=next(r for r in after['nodes'] if r['node_id']==NODE)
        require(target['binary_sha256']==BINARY_SHA,'wrong v1 runtime')
        d.save(out/'ACCEPTANCE.json',{'revision':revision,'source_commit':SOURCE,'sha256':BINARY_SHA,'gate':after,
               'original_rollout_state_unchanged':True,'mainnet_activated':False})
        print('V1_RECOVERY_AND_FIVE_NODE_PROGRESS_OK')
        print('REPORT='+str(out/'ACCEPTANCE.json'))
        print('DO_NOT_RUN_OLD_DEPLOY_OR_PUBLISH: fleet now contains the corrected reactor candidate')

if __name__=='__main__':
    try:main()
    except (Exception,KeyboardInterrupt) as e:
        print('STOP: '+str(e)+'; state and jobs retained; no automatic restart or rollback',file=sys.stderr)
        sys.exit(1)
