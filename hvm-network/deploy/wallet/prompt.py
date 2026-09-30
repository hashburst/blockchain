#!/usr/bin/env python3
"""Interactive offline wallet: passwords use an anonymous pipe, never argv/env."""
import argparse,getpass,json,subprocess,sys,tempfile,os
from pathlib import Path

def main():
 p=argparse.ArgumentParser();p.add_argument('action',choices=['create','encrypt','verify','sign-transfer']);p.add_argument('--binary',required=True);p.add_argument('--source');p.add_argument('--address');p.add_argument('--chain-id',type=int,choices=[4735490,4735489],required=True);p.add_argument('--out');p.add_argument('--draft');a=p.parse_args()
 if not sys.stdin.isatty():p.error('interactive terminal required')
 command=[a.binary,a.action,'--chain-id',str(a.chain_id)]
 if a.action!='create':
  if not a.source or not a.address:p.error('--source and --address required')
  command+=['--in',a.source,'--address',a.address]
 snapshot=None
 if a.action!='verify':
  if not a.out:p.error('--out required')
  command+=['--out',a.out]
 if a.action=='sign-transfer':
  if not a.draft:p.error('--draft required')
  snapshot=Path(a.draft).read_bytes()
  d=json.loads(snapshot)
  # No conversion to floating point or implicit wei/native-unit conversion.
  print('NATIVE HBT TRANSFER: 1 HBT = 100000000 native units')
  print(json.dumps({k:d.get(k) for k in ['chain_id','sender','to','value_units','sequence','compute_limit','max_fee_units']},indent=2))
  expected='SIGN '+str(a.chain_id)
  if input('Confirm by typing '+expected+': ')!=expected:raise SystemExit('Cancelled; no signature produced')
 password=getpass.getpass('Keystore password (at least 12 characters): ')
 if len(password)<12 or '\n' in password:raise SystemExit('Invalid password length/format')
 if a.action in ('create','encrypt') and getpass.getpass('Repeat password: ')!=password:raise SystemExit('Passwords differ')
 with tempfile.TemporaryDirectory(prefix='hashburst-sign-') as td:
  if snapshot is not None:
   # Sign the exact draft reviewed above, not a file replaced after confirmation.
   path=Path(td)/'draft.json'
   fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
   with os.fdopen(fd,'wb') as f:f.write(snapshot)
   command+=['--draft',str(path)]
  r=subprocess.run(command,input=password+'\n',text=True)
 raise SystemExit(r.returncode)
if __name__=='__main__':main()
