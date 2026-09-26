#!/usr/bin/env python3
"""Public RPC acceptance; optionally verify the exported real-wallet canary."""
import argparse,json,time,urllib.request
from pathlib import Path
RPC='https://blockchainapi.one/api/hashburst/hvm/testnet/evm'
def request(method,params=None):
 time.sleep(.3)
 req=urllib.request.Request(RPC,json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params or []}).encode(),{'Content-Type':'application/json'})
 with urllib.request.urlopen(req,timeout=30) as r:
  if r.status!=200:raise RuntimeError('HTTP failure')
  value=json.load(r)
 if value.get('jsonrpc')!='2.0' or value.get('id')!=1:raise RuntimeError('RPC envelope mismatch')
 return value
def rpc(method,params=None):
 d=request(method,params)
 if 'error' in d or 'result' not in d:raise RuntimeError(str(d))
 return d['result']
def require(condition,msg):
 if not condition:raise RuntimeError(msg)
def check_wallet(p):
 require(p.get('result')=='METAMASK_TESTNET_MANUAL_CANARY_OK','wallet test incomplete')
 require(p.get('chainId')==4735490 and p.get('endpoint')==RPC,'wallet network/endpoint mismatch')
 checks=p.get('checks',[])
 require([c['label'] for c in checks]==['transfer','deploy','call'],'three wallet transactions required')
 receipts=[]
 for c in checks:
  r=rpc('eth_getTransactionReceipt',[c['hash']]);require(r and r['status']=='0x1','unsuccessful receipt')
  for k in ('transactionHash','blockHash','blockNumber','status','gasUsed','contractAddress','logs'):
   require(r.get(k)==c['receipt'].get(k),'exported receipt differs: '+k)
  b=rpc('eth_getBlockByNumber',[r['blockNumber'],False]);require(b['hash']==r['blockHash'],'canonical block mismatch')
  require(int(r['gasUsed'],16)>0,'gas missing')
  tx=rpc('eth_getTransactionByHash',[c['hash']]);require(tx and tx['from'].lower()==p['account'].lower(),'wallet sender mismatch')
  require(int(tx['chainId'],16)==4735490,'transaction chain ID mismatch')
  receipts.append(r)
 deploy,call=receipts[1:]
 address=deploy['contractAddress'];require(address,'contract missing')
 require(rpc('eth_getCode',[address,'latest'])=='0x602a60005560006000a000','contract code mismatch')
 require(int(rpc('eth_getStorageAt',[address,'0x0','latest']),16)==42,'contract storage mismatch')
 logs=rpc('eth_getLogs',[{'blockHash':call['blockHash'],'address':address}])
 require(any(l['transactionHash']==call['transactionHash'] for l in logs),'canonical contract log missing')
 ws=p.get('websocket',{})
 require(ws.get('unsubscribed') is True,'unsubscribe evidence missing')
 require(ws.get('head',{}).get('result',{}).get('hash')==call['blockHash'],'newHeads proof mismatch')
 event=ws.get('log',{}).get('result',{})
 require(event.get('transactionHash')==call['transactionHash'] and event.get('blockHash')==call['blockHash'],'subscription log mismatch')
 require(any(all(event.get(k)==l.get(k) for k in ('address','data','topics','logIndex','transactionHash','blockHash')) for l in logs),'subscription does not match canonical log')
 return {'account':p['account'],'transactions':[c['hash'] for c in checks],'contract':address,'wallet_websocket_evidence_verified':True}
def main():
 a=argparse.ArgumentParser();a.add_argument('--wallet-proof');args=a.parse_args()
 require(rpc('eth_chainId')=='0x484202','not testnet')
 require(rpc('net_version')=='4735490','net_version mismatch')
 for method in ('admin_peers','personal_unlockAccount','eth_sendTransaction'):
  require('error' in request(method),'unapproved public method enabled: '+method)
 first=rpc('eth_getBlockByNumber',['finalized',False]);require(int(first['number'],16)>=53303,'activation not finalized')
 until=time.monotonic()+90
 while True:
  last=rpc('eth_getBlockByNumber',['finalized',False])
  if int(last['number'],16)>int(first['number'],16):break
  if time.monotonic()>until:raise RuntimeError('public finality stalled')
  time.sleep(3)
 proof={'ok':True,'chain_id':4735490,'endpoint':RPC,'height':int(last['number'],16),'block_hash':last['hash'],'timestamp':time.time()}
 name='GATE-public.json'
 if args.wallet_proof:
  proof.update(check_wallet(json.loads(Path(args.wallet_proof).read_text())));name='GATE-metamask.json'
 Path(name).write_text(json.dumps(proof,indent=2));print('PUBLIC_EVM_ACCEPTANCE_OK')
 if args.wallet_proof:print('METAMASK_RECEIPTS_LOGS_AND_SUBSCRIPTION_PROOF_OK')
 print('PROOF='+name)
if __name__=='__main__':main()
