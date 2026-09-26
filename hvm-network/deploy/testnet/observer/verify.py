import hashlib,importlib.util,json,os,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
s=importlib.util.spec_from_file_location('transport',R/'ssh-session.py');t=importlib.util.module_from_spec(s);s.loader.exec_module(t)
SOURCE='''import json,urllib.request
from pathlib import Path
def main(p):
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
 if p['action']=='health':
  with opener.open('http://127.0.0.1:18009/health',timeout=20) as r:d=json.load(r)
  if p.get('observer'):
   c=json.loads(Path('/etc/hashburst-hvm-testnet-ingress/node.json').read_text())
   if c['role']!='observer' or c['consensus_key_file'] or c['validator_id']:raise RuntimeError('observer has signing configuration')
   for name in ['consensus-bft-signatures.jsonl','consensus-votes.jsonl']:
    if (Path(c['data_dir'])/name).stat().st_size!=0:raise RuntimeError('observer signing journal is not empty')
  return d
 q=urllib.request.Request('http://127.0.0.1:18009/rpc',json.dumps(dict(jsonrpc='2.0',id=1,method='hb_getFinalizedCommitment',params=[p['height']])).encode(),{'Content-Type':'application/json'})
 with opener.open(q,timeout=20) as r:d=json.load(r)
 if 'error' in d:raise RuntimeError(str(d))
 return d['result']
'''
def main():
 os.umask(0o077);out=Path(tempfile.mkdtemp(prefix='observer-proof-',dir=R));sessions=[]
 try:
  for ip in ['64.31.4.9','77.90.188.153']:
   print('SSH_AUTH='+ip,flush=True);sessions.append(t.Session(t.ssh_command(ip),SOURCE))
  deadline=time.monotonic()+7200;first=None
  while time.monotonic()<deadline:
   try:
    h=[s.call({'action':'health','observer':i==0,'_timeout':45}) for i,s in enumerate(sessions)]
    for i,d in enumerate(h):
     if d.get('chain_id')!=4735490 or d.get('config_digest')!='502c051c2cf6040b988fe2bc94ae984ab7f709e059918237110af678266ea215' or d.get('role')!=('observer' if i==0 else 'validator'):raise ValueError('identity/network mismatch')
    print('FINALITY observer='+str(h[0]['finalized_height'])+' validator='+str(h[1]['finalized_height']),flush=True)
    height=min(d['finalized_height'] for d in h)
    if height>7 and abs(h[0]['finalized_height']-h[1]['finalized_height'])<=10:
     proofs=[s.call({'action':'proof','height':height,'_timeout':45}) for s in sessions]
     for p in proofs:
      if p.get('height')!=height or p.get('chain_id')!=4735490 or not p.get('certificate'):raise ValueError('invalid certificate/height')
     for k in ['hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root']:
      if not proofs[0].get(k) or proofs[0][k]!=proofs[1].get(k):raise ValueError('commitment mismatch '+k)
     (out/(str(height)+'.json')).write_text(json.dumps(proofs,indent=2))
     if first is not None and height>first:
      print('OBSERVER_FIXED_HEIGHT_AGREEMENT_OK\nOBSERVER_FINALITY_PROGRESS_OK\nOBSERVER_NO_SIGNING_VERIFIED\nLOGS='+str(out));return
     first=height
   except ValueError:raise
   except Exception as e:
    print('WAIT '+str(e),flush=True)
    if any(s.broken for s in sessions):raise
   time.sleep(15)
  raise RuntimeError('observer catch-up deadline exceeded; state retained')
 finally:
  for s in sessions:s.close()
if __name__=='__main__':main()
