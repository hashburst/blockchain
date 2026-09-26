#!/usr/bin/env python3
"""Compare commitments at a single finalized height over four operator tunnels.
Does not independently verify QC signatures; node validation remains trusted.
"""
import argparse,concurrent.futures,json,urllib.request
FIELDS=('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root')
def rpc(url,method,params):
 req=urllib.request.Request(url,json.dumps(dict(jsonrpc='2.0',id=1,method=method,params=params)).encode(),{'Content-Type':'application/json'})
 with urllib.request.urlopen(req,timeout=15) as r:d=json.load(r)
 if d.get('id')!=1 or 'error' in d:raise RuntimeError(str(d))
 return d['result']
def compare(rows,height):
 if len(rows)!=4:raise ValueError('four responses required')
 for r in rows:
  if r.get('chain_id')!=4735490 or r.get('height')!=height or not r.get('certificate'):
   raise ValueError('wrong identity/height or missing certificate')
  if any(not r.get(k) for k in FIELDS if k not in ('chain_id','height')):raise ValueError('missing commitment')
 if any(tuple(r[k] for k in FIELDS)!=tuple(rows[0][k] for k in FIELDS) for r in rows[1:]):
  raise ValueError('fixed-height commitments differ')
def main():
 p=argparse.ArgumentParser();p.add_argument('--rpc',action='append',required=True)
 p.add_argument('--out',required=True);a=p.parse_args()
 if len(set(a.rpc))!=4:p.error('four distinct RPC URLs required')
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
  heights=list(pool.map(lambda u:int(rpc(u,'hb_getFinalizedHeight',[]),16),a.rpc))
  height=min(heights)
  rows=list(pool.map(lambda u:rpc(u,'hb_getFinalizedCommitment',[height]),a.rpc))
 # Retain mismatching evidence as well as successful evidence.
 with open(a.out,'x') as f:json.dump(dict(height=height,urls=a.rpc,proofs=rows),f,indent=2)
 compare(rows,height)
 print('HVM_TESTNET_FIXED_HEIGHT_AGREEMENT_OK height='+str(height))
if __name__=='__main__':main()
