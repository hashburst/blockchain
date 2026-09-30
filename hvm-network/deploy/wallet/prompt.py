#!/usr/bin/env python3
"""Interactive offline wallet: passwords use an anonymous pipe, never argv/env."""
import argparse,getpass,json,subprocess,sys
from pathlib import Path

def main():
 p=argparse.ArgumentParser();p.add_argument('action',choices=['encrypt','verify','sign-transfer']);p.add_argument('--binary',required=True);p.add_argument('--source',required=True);p.add_argument('--address',required=True);p.add_argument('--chain-id',type=int,choices=[4735490,4735489],required=True);p.add_argument('--out');p.add_argument('--draft');a=p.parse_args()
 if not sys.stdin.isatty():p.error('interactive terminal required')
 command=[a.binary,a.action,'--in',a.source,'--address',a.address,'--chain-id',str(a.chain_id)]
 if a.action!='verify':
  if not a.out:p.error('--out required')
  command+=['--out',a.out]
 if a.action=='sign-transfer':
  if not a.draft:p.error('--draft required')
  d=json.loads(Path(a.draft).read_text())
  # No conversion to floating point or implicit wei/native-unit conversion.
  print('NATIVE HBT TRANSFER: 1 HBT = 100000000 native units')
  print(json.dumps({k:d.get(k) for k in ['chain_id','sender','to','value_units','sequence','compute_limit','max_fee_units']},indent=2))
  expected='SIGN '+str(a.chain_id)
  if input('Confirm by typing '+expected+': ')!=expected:raise SystemExit('Cancelled; no signature produced')
  command+=['--draft',a.draft]
 password=getpass.getpass('Keystore password (at least 12 characters): ')
 if len(password)<12 or '\n' in password:raise SystemExit('Invalid password length/format')
 if a.action=='encrypt' and getpass.getpass('Repeat password: ')!=password:raise SystemExit('Passwords differ')
 r=subprocess.run(command,input=password+'\n',text=True);raise SystemExit(r.returncode)
if __name__=='__main__':main()
