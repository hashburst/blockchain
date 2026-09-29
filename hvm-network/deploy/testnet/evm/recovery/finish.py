#!/usr/bin/env python3
"""Run the recovery, restart gate and public ingress in order; then hand off to MetaMask."""
import argparse,json,subprocess,sys
from pathlib import Path
R=Path(__file__).resolve().parent
def main():
 a=argparse.ArgumentParser();a.add_argument('--plan',required=True);a.add_argument('--start-prepared',action='store_true');args=a.parse_args()
 plan=str(Path(args.plan).resolve())
 def run(*words):subprocess.run([sys.executable,*words],cwd=R,check=True)
 action='start-prepared' if args.start_prepared else ('verify' if (R/'GATE-recover.json').exists() else 'recover')
 run('recover.py',action,'--plan',plan)
 run('recover.py','restart-v4','--plan',plan)
 if (R/'GATE-public.json').exists():run('verify-public.py')
 else:run('publish.py')
 print('TESTNET_PUBLIC_RUNTIME_READY')
 print('NEXT: open https://blockchainapi.one/hvm-testnet-metamask/ and follow README_IT.md sections 4-5.')
 print('REAL_WALLET_CONFIRMATIONS_REQUIRED_MAINNET_NOT_ACTIVATED')
if __name__=='__main__':main()
