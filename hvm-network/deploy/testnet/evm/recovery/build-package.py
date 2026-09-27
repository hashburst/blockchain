#!/usr/bin/env python3
"""Build the Linux recovery release from its reviewed repository commit."""
import argparse,hashlib,os,re,shutil,subprocess,tarfile
from pathlib import Path
R=Path(__file__).resolve().parent
ROOT=R.parents[4]
def main():
 a=argparse.ArgumentParser();a.add_argument('--out',required=True);a.add_argument('--commit',required=True);args=a.parse_args()
 if not re.fullmatch('[0-9a-f]{40}',args.commit):raise RuntimeError('full source commit required')
 if (ROOT/'.git').exists():
  head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
  if head!=args.commit or subprocess.check_output(['git','status','--porcelain'],cwd=ROOT,text=True).strip():raise RuntimeError('clean checkout at requested commit required')
 out=Path(args.out).resolve();out.mkdir(parents=True,exist_ok=True);dest=out/'HashBurst-EVM-Recovery-v1.0.2';dest.mkdir()
 env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',GOTOOLCHAIN='local')
 for name in ('hashburst-testnet','hvm-evm-gateway','hvm-evm-fund'):
  subprocess.run(['go','build','-p','1','-trimpath','-buildvcs=false','-o',str(dest/name),'./cmd/'+name],cwd=ROOT/'hvm-network',env=env,check=True)
 for name in ('exec-permissions.py','test-exec-permissions.py','inspect-live.py','resume-live.py','finish.py','recover.py','remote.py','offline.py','verify-public.py','publish.py','closeout.py','test-recovery.py','README_IT.md','TEST_RESULT.txt'):
  shutil.copy2(R/name,dest/name)
 for name in ('ssh-session.py','rollout.py','node-rollout.py','install-ingress.py','routes.conf','metamask-canary.html','test-rollout.py'):
  shutil.copy2(R.parent/name,dest/name)
 (dest/'SOURCE_COMMIT').write_text(args.commit+'\n')
 (dest/'BASE_COMMIT').write_text('febed851ecdc2bf725b2c1f3a57961ce463eb886\n')
 for rel in ('blockchain/consensus_recovery.go','blockchain/evm_recovery_transition_test.go','blockchain/open_existing.go','blockchain/replay_v2.go','internal/testnet/state.go','internal/testnet/migrate_evm.go','cmd/hvm-evm-fund/main.go','cmd/hvm-evm-fund/main_test.go'):
  target=dest/'source-changes'/rel;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(ROOT/'hvm-network'/rel,target)
 manifest=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+str(p.relative_to(dest))+'\n' for p in sorted(dest.rglob('*')) if p.is_file())
 (dest/'SHA256SUMS').write_text(manifest)
 archive=out/(dest.name+'.tar.gz')
 with tarfile.open(archive,'w:gz') as t:t.add(dest,arcname=dest.name)
 print('ARCHIVE='+str(archive));print('SHA256='+hashlib.sha256(archive.read_bytes()).hexdigest())
if __name__=='__main__':main()
