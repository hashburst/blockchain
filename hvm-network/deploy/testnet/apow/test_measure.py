import hashlib,importlib.util,json,struct,threading,unittest,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
def load(name,file):
 s=importlib.util.spec_from_file_location(name,R/file);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
m=load('measure_node','measure-node.py');c=load('measure_all','measure-all.py')
class MeasureTests(unittest.TestCase):
 def fixture(self):
  a='0x'+'12'*20
  j={'chain_id':4735490,'height':1,'parent_hash':'ab'*32,'poh':0,'bits':1,'epoch_start':0,'nonce':0}
  p=dict(j,author=a,beneficiary=a,signature='00'*65)
  while int.from_bytes(m.proof_digest(p),'big')>=1<<255:p['nonce']+=1
  return a,j,p
 def test_target_identity_and_job_binding(self):
  a,j,p=self.fixture();m.check_proof(p,j,a)
  for k,v in [('chain_id',4735489),('parent_hash','ff'*32),('beneficiary','0x'+'34'*20),('nonce',-1),('signature','x')]:
   with self.subTest(field=k),self.assertRaises(ValueError):m.check_proof(dict(p,**{k:v}),j,a)
 def test_weak_work_rejected(self):
  a,j,p=self.fixture()
  while int.from_bytes(m.proof_digest(p),'big')<1<<255:p['nonce']+=1
  with self.assertRaises(ValueError):m.check_proof(p,j,a)
 def test_handler_checks_real_digest_and_refuses_duplicate(self):
  a,j,p=self.fixture();s=m.ChallengeServer(j,a);t=threading.Thread(target=s.serve_forever);t.start()
  opener=urllib.request.build_opener(urllib.request.ProxyHandler({}));url='http://127.0.0.1:'+str(s.server_port)+s.token
  try:
   with opener.open(url,timeout=5) as r:self.assertEqual(json.load(r),j)
   req=urllib.request.Request(url,json.dumps(p).encode(),{'Content-Type':'application/json'})
   with opener.open(req,timeout=5) as r:self.assertTrue(json.load(r)['synthetic_only'])
   with self.assertRaises(urllib.error.HTTPError):opener.open(req,timeout=5)
   self.assertGreater(s.elapsed,0)
  finally:s.shutdown();s.server_close();t.join()
 def test_runtime_bounds_and_separate_key(self):
  cmd=m.command('hvm-apow-measure-test','http://127.0.0.1:1234/test')
  for arg in ('--property=CPUQuota=25%','--property=RuntimeMaxSec=20','--property=IPAddressDeny=any','--property=IPAddressAllow=localhost','--property=User=hashburst-apow-miner'):self.assertIn(arg,cmd)
  self.assertNotIn('--loop',cmd);self.assertNotIn('consensus',str(m.KEY))
 def test_proposal_not_activation_plan(self):
  rows=[{'node_id':str(i),'address':'0x'+str(i),'measured_hashes_per_second':100000,'samples':[{'status':'success'}]} for i in range(4)]
  p=c.summarize(rows);self.assertIsNone(p['activation_height']);self.assertEqual(p['proposed_initial_bits'],16)
  rows[0]['measured_hashes_per_second']=float('nan')
  with self.assertRaises(ValueError):c.summarize(rows)
 def test_remote_source_compiles(self):
  compile(c.pre.SOURCE+'\ninspect=main\n'+(R/'stage-node.py').read_text()+'\n'+(R/'measure-node.py').read_text(),'remote','exec')
if __name__=='__main__':unittest.main()
