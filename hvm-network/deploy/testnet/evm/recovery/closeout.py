#!/usr/bin/env python3
"""Assemble evidence and optionally publish it to the existing PR. Never merges."""
import argparse,hashlib,json,subprocess,time
from pathlib import Path
R=Path(__file__).resolve().parent
def main():
 a=argparse.ArgumentParser();a.add_argument('--github',action='store_true');args=a.parse_args()
 sha=hashlib.sha256((R/'hashburst-testnet').read_bytes()).hexdigest()
 results={}
 for kind in ('restart-v4','public','metamask'):
  p=R/('GATE-'+kind+'.json');v=json.loads(p.read_text())
  if v.get('ok') is not True or v.get('chain_id')!=4735490 or v.get('height',0)<53303:raise RuntimeError('invalid gate '+kind)
  if kind=='restart-v4' and v.get('binary_sha256')!=sha:raise RuntimeError('binary gate mismatch')
  if time.time()-v['timestamp']>86400:raise RuntimeError('evidence older than 24h: rerun corresponding read-only checks')
  results[kind]=v
 report={'source_commit':(R/'SOURCE_COMMIT').read_text().strip(),'binary_sha256':sha,'testnet':4735490,'legacy':1337,'mainnet_not_activated':4735489,'gates':results}
 (R/'TESTNET_CLOSEOUT.json').write_text(json.dumps(report,indent=2))
 body='Testnet EVM recovery and public acceptance evidence (operator run).\n\n```json\n'+json.dumps(report,indent=2)+'\n```\n\nThe MetaMask gate is a manual wallet run corroborated against public canonical receipts/logs. It is not universal Ethereum compatibility certification. Mainnet is not activated. Consensus/economics review and required GitHub checks remain merge gates.\n'
 (R/'PR_EVIDENCE.md').write_text(body)
 if args.github:
  subprocess.run(['gh','pr','comment','19','--repo','hashburst/blockchain','--body-file',str(R/'PR_EVIDENCE.md')],check=True)
 print('TESTNET_EVIDENCE_ASSEMBLED_MAINNET_NOT_ACTIVATED')
if __name__=='__main__':main()
