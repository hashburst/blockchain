#!/usr/bin/env python3
"""Repair the proven EXEC permission failure, then verify; never repeat migration."""
import argparse,subprocess,sys
from pathlib import Path
R=Path(__file__).resolve().parent
def main():
 a=argparse.ArgumentParser();a.add_argument('--plan',required=True);args=a.parse_args();plan=str(Path(args.plan).resolve())
 steps=[['recover.py','repair-exec','--plan',plan],['recover.py','restart-v4','--plan',plan]]
 steps.append(['verify-public.py'] if (R/'GATE-public.json').exists() else ['publish.py'])
 for step in steps:
  print('STEP='+' '.join(step),flush=True)
  r=subprocess.run([sys.executable]+step,cwd=R)
  if r.returncode:
   print('STOP: inspect the preceding diagnostics. No automatic rollback or migration retry.',flush=True);return r.returncode
 print('TESTNET_PUBLIC_RUNTIME_READY_REAL_METAMASK_CONFIRMATIONS_REQUIRED')
 return 0
if __name__=='__main__':sys.exit(main())
