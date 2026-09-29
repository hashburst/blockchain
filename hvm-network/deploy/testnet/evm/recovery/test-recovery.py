from types import SimpleNamespace
import copy,hashlib,importlib.util,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
R=Path(__file__).resolve().parent
def module(name):
 spec=importlib.util.spec_from_file_location(name,R/(name+'.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
offline=module('offline');recover=module('recover');public=module('verify-public');resume=module('resume-live')
class Guards(unittest.TestCase):
 def setUp(self):
  self.evm={'activation_height':53303,'gas_limit':200000,'base_fee_wei':1}
  self.plan={'chain_id':4735490,'binary_sha256':recover.ORIGINAL,'observer_node_id':'hvm-testnet-ingress','evm':self.evm}
  self.cfg={'node_id':'hvm-testnet-v1','role':'validator','protocol':{'chain_id':4735490},'peer_id':'peer','genesis_hash':'genesis'}
  self.job={'node_id':'hvm-testnet-v1','role':'validator','evm':self.evm}
  self.proof={'binary_sha256':recover.ORIGINAL,'chain_id':4735490,'evm':self.evm,'identity':'peer','genesis':'genesis'}
 def test_resume_stops_before_restart_if_repair_or_finality_fails(self):
  with patch('sys.argv',['resume-live.py','--plan','plan.json']),patch.object(resume.subprocess,'run',return_value=SimpleNamespace(returncode=2)) as run:
   self.assertEqual(resume.main(),2);self.assertEqual(run.call_count,1);self.assertEqual(run.call_args.args[0][1:3],['recover.py','repair-exec'])
 def test_resume_never_repeats_migration(self):
  with patch('sys.argv',['resume-live.py','--plan','plan.json']),patch.object(resume.subprocess,'run',return_value=SimpleNamespace(returncode=0)) as run:
   self.assertEqual(resume.main(),0)
   steps=[c.args[0][1:] for c in run.call_args_list]
   self.assertEqual(steps[0][0:2],['recover.py','repair-exec']);self.assertEqual(steps[1][0:2],['recover.py','restart-v4'])
   self.assertFalse(any('start-prepared' in s or 'recover' in s for s in steps))
 def test_closed_transport_is_fatal(self):
  recover.require_live_transports({'v1':SimpleNamespace(broken=False)})
  with self.assertRaisesRegex(RuntimeError,'SSH_TRANSPORT_LOST'):
   recover.require_live_transports({'v1':SimpleNamespace(broken=True)})
 def test_original_plan_only(self):
  recover.validate_plan(self.plan)
  for key,value in [('chain_id',4735489),('binary_sha256','replacement'),('evm',dict(self.evm,activation_height=60000))]:
   bad=copy.deepcopy(self.plan);bad[key]=value
   with self.assertRaises(RuntimeError):recover.validate_plan(bad)
 def test_original_and_partial_migration(self):
  offline.validate_proof(self.cfg,self.proof,self.job)
  self.cfg['protocol']['evm']=self.evm;offline.validate_proof(self.cfg,self.proof,self.job)
  self.cfg['protocol']['evm']=dict(self.evm,gas_limit=300000)
  with self.assertRaises(RuntimeError):offline.validate_proof(self.cfg,self.proof,self.job)
 def test_identity_and_observer_signing(self):
  for key in ('peer_id','genesis_hash','node_id'):
   c=copy.deepcopy(self.cfg);c[key]='changed'
   with self.assertRaises(RuntimeError):offline.validate_proof(c,self.proof,self.job)
  self.cfg.update(role='observer',consensus_key_file='/private');self.job['role']='observer'
  with self.assertRaises(RuntimeError):offline.validate_proof(self.cfg,self.proof,self.job)
 def test_inventory_detects_each_mutation(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)
   for n in offline.FILES:(p/n).write_bytes(b'original')
   before=offline.inventory(p)
   for n in offline.FILES:
    (p/n).write_bytes(b'mutated');self.assertNotEqual(before,offline.inventory(p));(p/n).write_bytes(b'original')
   self.assertEqual(before,offline.inventory(p))
 def test_running_node_never_migrates(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d);self.cfg['data_dir']=d
   (p/'cfg.json').write_text(json.dumps(self.cfg));(p/'evm-rollout-proof.json').write_text(json.dumps(self.proof));(p/'binary').write_bytes(b'binary')
   self.job.update(config=str(p/'cfg.json'),binary=str(p/'binary'),sha256=offline.sha(p/'binary'),service='test.service')
   (p/'job.json').write_text(json.dumps(self.job))
   with patch.object(offline.subprocess,'check_output',return_value='active\n'),patch.object(offline.subprocess,'run') as run:
    with self.assertRaisesRegex(RuntimeError,'stopped'):offline.main(p/'job.json')
    run.assert_not_called()
 def test_altered_journal_never_migrates(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d);self.cfg['data_dir']=d
   name='consensus-bft-signatures.jsonl';(p/name).write_bytes(b'changed')
   self.proof['journals']={name:{'length':7,'sha256':hashlib.sha256(b'original').hexdigest()}}
   (p/'cfg.json').write_text(json.dumps(self.cfg));(p/'evm-rollout-proof.json').write_text(json.dumps(self.proof));(p/'binary').write_bytes(b'binary')
   self.job.update(config=str(p/'cfg.json'),binary=str(p/'binary'),sha256=offline.sha(p/'binary'),service='test.service')
   (p/'job.json').write_text(json.dumps(self.job))
   with patch.object(offline.subprocess,'check_output',return_value='inactive\n'),patch.object(offline.subprocess,'run') as run:
    with self.assertRaisesRegex(RuntimeError,'journal changed'):offline.main(p/'job.json')
    run.assert_not_called()
 def test_wallet_missing_or_wrong_chain(self):
  with self.assertRaises(RuntimeError):public.check_wallet({})
  with self.assertRaises(RuntimeError):public.check_wallet({'result':'METAMASK_TESTNET_MANUAL_CANARY_OK','chainId':4735489,'endpoint':public.RPC})
if __name__=='__main__':unittest.main()
