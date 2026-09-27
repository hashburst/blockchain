import contextlib,hashlib,importlib.util,io,json,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('rollout',R/'rollout.py');roll=importlib.util.module_from_spec(spec);spec.loader.exec_module(roll)
class Guards(unittest.TestCase):
 def test_agreement_and_activation(self):
  p={'chain_id':4735490,'height':51473,'hash':'a','certificate':{'present':True}}
  roll.compare([p.copy() for _ in range(5)],51473)
  with self.assertRaisesRegex(RuntimeError,'disagree'):roll.compare([p,{**p,'hash':'b'}],51473)
  with self.assertRaisesRegex(RuntimeError,'missing EVM'):roll.compare([{**p,'height':53303}],53303)
 def exercise(self,action='apply',updated=False,fail_stop=False):
  events=[];counter=[53400];idx=[0];new=hashlib.sha256(b'binary').hexdigest()
  class Session:
   broken=False
   def __init__(self,*args):self.i=idx[0];idx[0]+=1;self.sha=new if updated else roll.OLD
   def close(self):pass
   def call(self,p):
    a=p['action'];events.append((self.i,a));n=p['node']
    if a=='inspect':return {'sha256':self.sha,'prepared':updated}
    if a=='status':
     counter[0]+=1
     return {'chain_id':4735490,'config_digest':'same','node_id':n['node_id'],'reactor_running':True,'peer_count':4,'finalized_height':counter[0]}
    if a=='proof':return {'chain_id':4735490,'height':p['height'],'hash':str(p['height']),'certificate':{'present':True},'evm_state_root':'a','evm_receipts_root':'b'}
    if a=='stop' and fail_stop:raise RuntimeError('STOP_PROOF_FAILURE')
    if a=='start':self.sha=new
    return {}
  with tempfile.TemporaryDirectory() as td:
   r=Path(td);(r/'hashburst-testnet').write_bytes(b'binary');(r/'node.py').write_text('pass');(r/'SHA256SUMS').write_text(new+'  hashburst-testnet\n')
   with patch.object(roll,'R',r),patch.object(roll.ssh,'Session',Session),patch.object(roll.time,'sleep',lambda _:None),patch.object(sys,'argv',['rollout.py',action]),contextlib.redirect_stdout(io.StringIO()):
    try:roll.main()
    except RuntimeError:
     if not fail_stop:raise
  return events
 def test_rolling_order(self):
  events=self.exercise();mutations=[e for e in events if e[1] in ('stage','stop','install','start')]
  self.assertEqual(mutations,[(i,a) for i in range(5) for a in ('stage','stop','install','start')])
 def test_completed_nodes_not_restarted(self):
  self.assertFalse(any(a in ('stage','stop','install','start') for _,a in self.exercise(updated=True)))
 def test_verify_read_only(self):
  self.assertFalse(any(a in ('stage','stop','install','start','restart') for _,a in self.exercise('verify',updated=True)))
 def test_failed_stop_does_not_install_or_continue(self):
  mutations=[e for e in self.exercise(fail_stop=True) if e[1] in ('stage','stop','install','start')]
  self.assertEqual(mutations,[(0,'stage'),(0,'stop')])
 def test_readiness(self):
  self.assertFalse(roll.ready([{'chain_id':4735490,'reactor_running':False,'peer_count':4}]))
if __name__=='__main__':unittest.main()
