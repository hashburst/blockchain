#!/usr/bin/env python3
import concurrent.futures,hashlib,importlib.util,json,os,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location("ssh_session",ROOT/"ssh-session.py")
transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)

def main():
    os.umask(0o077)
    for line in (ROOT/'SHA256SUMS').read_text().splitlines():
        sha,name=line.split('  ',1)
        if hashlib.sha256((ROOT/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('package checksum mismatch: '+name)
    targets=json.loads((ROOT/'targets.json').read_text())
    logs=Path(tempfile.mkdtemp(prefix='resume-results-',dir=ROOT))
    script=(ROOT/'node-action.py').read_text()
    connections={}
    def call(n,action):
        payload={'node':n,'action':action,'_timeout':650 if action=='restart' else 30,'config':json.loads((ROOT/(n['node_id']+'.validator.json')).read_text())}
        stamp=time.time_ns()
        log=logs/(str(stamp)+'-'+action+'-'+n['node_id']+'.log')
        try:
            result=connections[n['node_id']].call(payload)
            log.write_text(json.dumps(result,indent=2))
            return result
        except Exception as exc:
            log.write_text(str(exc))
            raise RuntimeError(n['node_id']+' '+action+': '+str(exc)) from exc
    def batch(action):
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            futures=[pool.submit(call,n,action) for n in targets]
            return [f.result() for f in futures]
    highest={}
    def progress(goal,timeout=240):
        end=time.monotonic()+timeout;last='not ready'
        while time.monotonic()<end:
            try:
                rows=batch('status')
                for r in rows:
                    node_id=r['node_id'];height=r['finalized_height']
                    if height<highest.get(node_id,0):
                        raise ValueError('finalized height regressed: '+node_id)
                    highest[node_id]=height
                last=' '.join(r['node_id']+'='+str(r['finalized_height']) for r in rows)
                print('FINALITY '+last,flush=True)
                if all(r['finalized_height']>=goal and r['peer_count']>=3 for r in rows):
                    roots={r['consensus']['next_validator_set_root'] for r in rows}
                    if len(roots)!=1:raise RuntimeError('validator roots differ')
                    return rows
            except ValueError:raise
            except Exception as exc:last=str(exc);print('WAIT '+last[:240],flush=True)
            time.sleep(3)
        raise RuntimeError('Finality gate timeout: '+last)
    try:
        for n in targets:
            print('SSH_AUTH='+n['ip'],flush=True)
            connections[n['node_id']]=transport.Session(transport.ssh_command(n['ip']),script)
            print('DEDICATED_SSH_READY='+n['ip'],flush=True)
        first=progress(8)
        # Require a second later finalized height on every validator.
        goal=max(r['finalized_height'] for r in first)+3
        rows=progress(goal)
        (logs/'finality-before-restart.json').write_text(json.dumps(rows,indent=2))
        print('HVM_TESTNET_FINALITY_PROGRESS_OK',flush=True)
        proof=call(targets[3],'restart')
        (logs/'restart-proof.json').write_text(json.dumps(proof,indent=2))
        print('SINGLE_VALIDATOR_RESTARTED=hvm-testnet-v4',flush=True)
        after=progress(max(proof['max_height'],goal)+3)
        end=time.monotonic()+90
        while True:
            try: recovery=call(targets[3],'recovery');break
            except Exception:
                if time.monotonic()>end:raise
                time.sleep(3)
        (logs/'recovery-proof.json').write_text(json.dumps(recovery,indent=2))
        (logs/'finality-after-restart.json').write_text(json.dumps(after,indent=2))
        print('HVM_TESTNET_VALIDATORS_OK=4\nHVM_TESTNET_FINALITY_PROGRESS_OK\nHVM_TESTNET_SINGLE_NODE_RESTART_RECOVERY_OK\nJOURNAL_PREFIX_PRESERVED\nNEW_PRECOMMIT_VERIFIED\nLOGS='+str(logs),flush=True)
    except Exception:
        print('RESUME_STOPPED: current node state and journals retained; no automatic rollback. LOGS='+str(logs),flush=True)
        raise
    finally:
        for session in connections.values():session.close()
if __name__=='__main__':
    try:main()
    except KeyboardInterrupt:print('INTERRUPTED: node state retained.')
    except Exception as exc:print('ERROR: '+str(exc));raise SystemExit(1)
