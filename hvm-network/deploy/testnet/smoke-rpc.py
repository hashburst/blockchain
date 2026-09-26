#!/usr/bin/env python3
"""Read-only JSON-RPC smoke check; no transaction or node mutation."""
import argparse,json,time,urllib.request
def main():
 p=argparse.ArgumentParser()
 p.add_argument('--url',required=True,help='Exact private or public RPC URL')
 p.add_argument('--seconds',type=int,default=15)
 a=p.parse_args()
 if not 1<=a.seconds<=60:p.error('seconds must be between 1 and 60')
 def rpc(method):
  req=urllib.request.Request(a.url,json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':[]}).encode(),{'Content-Type':'application/json'})
  with urllib.request.urlopen(req,timeout=10) as response:d=json.load(response)
  if d.get('jsonrpc')!='2.0' or d.get('id')!=1 or 'error' in d:raise RuntimeError(str(d))
  return d['result']
 if rpc('eth_chainId')!='0x484202':raise RuntimeError('wrong testnet chain ID')
 before=int(rpc('hb_getFinalizedHeight'),16)
 time.sleep(a.seconds)
 after=int(rpc('hb_getFinalizedHeight'),16)
 print(json.dumps({'before':before,'after':after}))
 if type(before)!=int or type(after)!=int or after<=before:raise RuntimeError('finality did not advance')
 print('HVM_TESTNET_RPC_READ_ONLY_SMOKE_OK')
if __name__=='__main__':main()
