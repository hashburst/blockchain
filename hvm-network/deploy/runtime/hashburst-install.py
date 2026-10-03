#!/usr/bin/env python3
"""Resumable TESTNET runtime update. Never converts ledger files or provisions mainnet.
Run on a target VPS. install starts a persistent systemd job; resume advances only
pre-start phases. Once start is requested, recovery uses verify without restarting.
Release JSON requires chain_id, source_commit, sha256, predecessors (SHA256 list).
"""
import argparse, fcntl, hashlib, json, os, re, shutil, subprocess, sys, tempfile, time, urllib.request
from pathlib import Path
NODES = ['hvm-testnet-v'+str(i) for i in range(1,5)] + ['hvm-testnet-ingress']

def require(condition, message):
    if not condition: raise RuntimeError(message)

def digest(path, size=None):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        remaining=size
        while remaining is None or remaining:
            b=f.read(1048576 if remaining is None else min(remaining,1048576))
            if not b:
                require(remaining in (None,0),'truncated evidence')
                break
            h.update(b)
            if remaining is not None: remaining-=len(b)
    return h.hexdigest()

def atomic(path, value):
    fd, name=tempfile.mkstemp(prefix='.state-',dir=path.parent)
    try:
        with os.fdopen(fd,'w') as f:
            json.dump(value,f,sort_keys=True); f.flush(); os.fsync(f.fileno())
        os.replace(name,path)
        d=os.open(path.parent,os.O_RDONLY)
        try: os.fsync(d)
        finally: os.close(d)
    finally:
        if os.path.exists(name):os.unlink(name)

def run(*args):
    try:
        return subprocess.run(args,check=True,capture_output=True,text=True,timeout=30).stdout.strip()
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f'{args[0]} exited {e.returncode}: {(e.stderr or e.stdout or str(e)).strip()}') from e

def props(unit):
    return dict(line.split('=',1) for line in run('systemctl','show',unit,'--property=ActiveState,SubState,MainPID,ExecStart,User,RootDirectory,RootImage').splitlines() if '=' in line)

def effective(p,binary,cfg):
    return p['ExecStart'].count('path=')==1 and 'path='+str(binary)+' ;' in p['ExecStart'] and 'argv[]='+str(binary)+' --config '+str(cfg)+' ;' in p['ExecStart']

def manifest(path):
    r=json.loads(path.read_text())
    require(r['chain_id']==4735490,'only testnet runtime updates are implemented')
    require(re.fullmatch('[0-9a-f]{40}',r['source_commit']) is not None,'invalid source commit')
    require(re.fullmatch('[0-9a-f]{64}',r['sha256']) is not None,'invalid binary digest')
    require(isinstance(r['predecessors'],list) and r['predecessors'] and all(isinstance(x,str) and re.fullmatch('[0-9a-f]{64}',x) for x in r['predecessors']),'reviewed predecessor digests required')
    return r

def prefix(data):
    out={}
    for name in ('consensus-votes.jsonl','consensus-bft-signatures.jsonl'):
        p=data/name;require(p.is_file() and not p.is_symlink(),'journal missing or symlink')
        n=p.stat().st_size;out[name]={'size':n,'sha256':digest(p,n)}
    return out

def preserved(record,cfg,data):
    require(digest(cfg)==record['config'],'config changed')
    require(digest(data/'runtime.pin')==record['pin'],'identity pin changed')
    for name,p in record['journals'].items():require(digest(data/name,p['size'])==p['sha256'],'journal prefix changed')

def verify(unit,cfg,c,binary,record,timeout):
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}));end=time.monotonic()+timeout;first=None
    while time.monotonic()<end:
        p=props(unit);require(effective(p,binary,cfg),'effective ExecStart differs')
        require(p['ActiveState']!='failed','service failed; inspect journal, no restart')
        # During activation MainPID may refer to a pre-exec systemd child.
        # Do not accept health or inspect that transient executable as the node.
        if p['ActiveState'] != 'active' or p.get('SubState') != 'running':
            time.sleep(5);continue
        pid=int(p['MainPID'])
        require(pid>0,'active service without managed process')
        observed=digest('/proc/'+str(pid)+'/exe')
        confirmed=props(unit)
        if (confirmed['MainPID'],confirmed['ActiveState'],confirmed.get('SubState')) != (str(pid),'active','running'):
            time.sleep(5);continue
        require(effective(confirmed,binary,cfg),'effective ExecStart differs')
        require(observed==record['sha256'],'running binary differs')
        try:
            with opener.open('http://127.0.0.1:18009/health',timeout=5) as f:h=json.load(f)
            require(h['chain_id']==4735490 and h['node_id']==c['node_id'] and h['peer_id']==c['peer_id'] and h['role']==c['role'],'health identity mismatch')
            require(pid>0,'health without managed process')
            if h.get('reactor_running') and h.get('peer_count',0)>0:
                if first is None:first=h['finalized_height']
                if h['finalized_height']>first:
                    preserved(record,cfg,Path(c['data_dir']));return h
        except (OSError,ValueError) as e:print('WAIT '+str(e),flush=True)
        time.sleep(5)
    raise RuntimeError('verification deadline; repeat verify, not install; APoW activation barrier may require separate coordinated miner acceptance')

def main():
    a=argparse.ArgumentParser(description=__doc__)
    a.add_argument('action',choices=['install','resume','verify','status','worker'])
    a.add_argument('--node',choices=NODES,required=True)
    a.add_argument('--release',type=Path,required=True);a.add_argument('--binary',type=Path,required=True)
    a.add_argument('--timeout',type=int,default=7200);args=a.parse_args()
    require(os.geteuid()==0,'run on target VPS as root');require(0<args.timeout<=86400,'timeout out of bounds')
    r=manifest(args.release);sha=r['sha256'];require(digest(args.binary)==sha,'binary checksum mismatch')
    stem='hashburst-hvm-testnet'+('-ingress' if args.node.endswith('ingress') else '')
    unit=stem+'.service';cfg=Path('/etc')/stem/'node.json';c=json.loads(cfg.read_text())
    require(c['network']=='testnet' and c['protocol']['chain_id']==4735490 and c['node_id']==args.node,'configuration network/identity differs')
    require(c['role']==('observer' if args.node.endswith('ingress') else 'validator'),'role differs')
    require(c['role']!='observer' or not c.get('consensus_key_file'),'observer has signing key configured')
    data=Path(c['data_dir']);require(data.resolve()==data and data.is_dir(),'canonical existing data directory required')
    root=Path('/var/lib/hashburst-runtime-installer')/args.node/sha
    root.mkdir(parents=True,exist_ok=True,mode=0o700);require(root.resolve()==root,'state symlink')
    state=root/'state.json';binary=Path('/opt/hashburst-runtime-installer')/sha/'hashburst-testnet'
    job='hvm-runtime-'+args.node+'-'+sha[:16]
    if args.action=='status':
        print(state.read_text() if state.exists() else '{"phase":"not_started"}');return
    # This lock guards request publication; the worker takes its own execution lock.
    with (root.parent/('worker.lock' if args.action in ('worker','verify') else 'request.lock')).open('a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        if args.action in ('install','resume'):
            require(not args.release.is_symlink() and not args.binary.is_symlink(),'release files must not be symlinks')
            for source,name in ((Path(__file__),'installer.py'),(args.release,'release.json'),(args.binary,'candidate')):
                target=root/name
                if target.exists():require(digest(target)==digest(source),'immutable job artifact changed')
                else:
                    with source.open('rb') as src,target.open('xb') as dst:shutil.copyfileobj(src,dst);dst.flush();os.fsync(dst.fileno())
            current=run('systemctl','show',job+'.service','--property=ActiveState','--value')
            if current in ('active','activating'):print('PERSISTENT_JOB_ALREADY_RUNNING='+job);return
            if current not in ('','inactive','failed'):raise RuntimeError('unexpected job state')
            # Same intent is safe to resume: phase record determines the next step.
            run('systemd-run','--no-block','--unit='+job,'--collect','--property=Type=oneshot','--property=TimeoutStartSec=infinity',sys.executable,str(root/'installer.py'),'worker','--node',args.node,'--release',str(root/'release.json'),'--binary',str(root/'candidate'),'--timeout',str(args.timeout))
            print('PERSISTENT_JOB_REQUESTED='+job);return
        record=json.loads(state.read_text()) if state.exists() else None
        if args.action=='verify':
            require(record is not None and record['phase'] in ('start_requested','verified'),'no start intent to verify')
            h=verify(unit,cfg,c,binary,record,args.timeout);record.update(phase='verified',health=h);atomic(state,record);print('RUNTIME_VERIFIED_NO_RESTART');return
        p=props(unit);require(not p['RootDirectory'] and not p['RootImage'],'service namespace requires review')
        if record is None:
            match=re.search(r'path=([^ ;]+)',p['ExecStart']);require(match is not None,'cannot resolve predecessor')
            old=match.group(1);require(digest(old) in r['predecessors'],'unexpected predecessor')
            require(effective(p,old,cfg),'unexpected predecessor command')
            pid=int(p['MainPID']);require(pid>0 and digest('/proc/'+str(pid)+'/exe')==digest(old),'predecessor is not running')
            record={'phase':'prepared','node':args.node,'sha256':sha,'config':digest(cfg),'old':old,'old_sha256':digest(old)};atomic(state,record)
        if record['phase']=='prepared':
            require(digest(cfg)==record['config'],'configuration changed')
            p=props(unit);pid=int(p['MainPID'])
            if pid:require(digest('/proc/'+str(pid)+'/exe')==record['old_sha256'],'unexpected process before stop')
            run('systemctl','stop','--no-block',unit)
            end=time.monotonic()+120
            while int(props(unit)['MainPID']) or props(unit)['ActiveState'] not in ('inactive','failed'):
                require(time.monotonic()<end,'stop pending; resume later');time.sleep(2)
            record.update(phase='stopped',pin=digest(data/'runtime.pin'),journals=prefix(data));atomic(state,record)
        if record['phase']=='stopped':
            require(int(props(unit)['MainPID'])==0,'service restarted externally')
            preserved(record,cfg,data)
            binary.parent.mkdir(parents=True,exist_ok=True,mode=0o755);require(binary.parent.resolve()==binary.parent,'binary directory symlink')
            if not binary.exists():shutil.copyfile(args.binary,binary)
            require(not binary.is_symlink() and digest(binary)==sha,'installed binary differs');binary.chmod(0o755)
            run('runuser','-u',p['User'],'--','test','-x',str(binary))
            drop=Path('/etc/systemd/system')/(unit+'.d')/'zzzz-hvm-runtime.conf';drop.parent.mkdir(parents=True,exist_ok=True)
            text='[Service]\nExecStart=\nExecStart='+str(binary)+' --config '+str(cfg)+'\nExecPaths='+str(binary)+'\n'
            if drop.exists():require(drop.read_text()==text,'existing runtime override differs')
            else:
                with drop.open('x') as f:f.write(text);f.flush();os.fsync(f.fileno())
            run('systemctl','daemon-reload');require(effective(props(unit),binary,cfg),'override shadowed; retained stopped')
            record['phase']='configured';atomic(state,record)
        if record['phase']=='configured':
            preserved(record,cfg,data);require(effective(props(unit),binary,cfg),'configured command differs')
            record['phase']='start_requested';atomic(state,record)
            run('systemctl','start','--no-block',unit)
        require(record['phase'] in ('start_requested','verified'),'unknown phase')
        h=verify(unit,cfg,c,binary,record,args.timeout);record.update(phase='verified',health=h);atomic(state,record)
        print('RUNTIME_VERIFIED_NO_LEDGER_CONVERSION_NO_MINER_STARTED')
if __name__=='__main__':
    try:main()
    except Exception as e:raise SystemExit('STOP: '+str(e)+'; state retained, no rollback')
