#!/usr/bin/env python3
import argparse,hashlib,os,shutil,subprocess,tarfile
from pathlib import Path
R=Path(__file__).resolve().parent
ROOT=R.parents[3]
def main():
 a=argparse.ArgumentParser();a.add_argument('--out',required=True);args=a.parse_args()
 commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
 if subprocess.check_output(['git','status','--porcelain'],cwd=ROOT,text=True).strip():raise RuntimeError('clean checkout required')
 out=Path(args.out).resolve();dest=out/'HashBurst-HVM-Pacemaker-v1.0.0';dest.mkdir(parents=True)
 for p in R.rglob('*'):
  if p.is_file() and '__pycache__' not in p.parts:
   target=dest/p.relative_to(R);target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,target)
 env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',GOTOOLCHAIN='local')
 subprocess.run(['go','build','-p','1','-trimpath','-buildvcs=false','-o',str(dest/'hashburst-testnet'),'./cmd/hashburst-testnet'],cwd=ROOT/'hvm-network',env=env,check=True)
 (dest/'SOURCE_COMMIT').write_text(commit+'\n')
 (dest/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(dest).as_posix()+'\n' for p in sorted(dest.rglob('*')) if p.is_file()))
 archive=out/(dest.name+'.tar.gz')
 with tarfile.open(archive,'w:gz') as t:t.add(dest,arcname=dest.name)
 print(archive)
if __name__=='__main__':main()
