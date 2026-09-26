import importlib.util,json,unittest
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).parent
spec=importlib.util.spec_from_file_location('node',ROOT/'node-action.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class ResumeTests(unittest.TestCase):
 def setUp(self):
  self.c={'node_id':'v1','peer_id':'peer'}
  self.d=dict(ok=True,network='testnet',chain_id=4735490,node_id='v1',peer_id='peer',config_digest=m.DIGEST,role='validator',finalized_height=23219,reactor_running=True,reactor_status_fresh=False,reactor={'running':True})
 def test_busy_snapshot_does_not_mean_stopped(self):
  with patch.object(m,'get',return_value=self.d):
   self.assertEqual(m.health(self.c,'validator')['finalized_height'],23219)
 def test_guards_preserved(self):
  for key,value in [('chain_id',1337),('role','observer'),('config_digest','wrong'),('peer_id','wrong'),('reactor_running',False),('finalized_height',True),('reactor',{'evidence_count':1})]:
   with self.subTest(key=key),patch.object(m,'get',return_value=dict(self.d,**{key:value})):
    with self.assertRaises(RuntimeError):m.health(self.c,'validator')
 def test_no_activation_actions(self):
  for action in ['prepare','promote','start','preflight']:
   with self.assertRaisesRegex(RuntimeError,'resume action not allowed'):
    m.main({'node':{},'config':{},'action':action})
 def test_only_v4_restart(self):
  with self.assertRaisesRegex(RuntimeError,'only v4'):
   m.main({'node':{'node_id':'hvm-testnet-v1'},'config':{},'action':'restart'})
if __name__=='__main__':unittest.main(verbosity=2)
