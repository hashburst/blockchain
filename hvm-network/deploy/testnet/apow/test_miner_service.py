import importlib.util,unittest
from pathlib import Path
s=importlib.util.spec_from_file_location('miner_service',Path(__file__).with_name('miner-service.py'))
m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Guards(unittest.TestCase):
 def test_mainnet_and_observer_refused(self):
  c={'network':'testnet','protocol':{'chain_id':4735490},'role':'validator','node_id':'v1'}
  h={'chain_id':4735490,'role':'validator','node_id':'v1'}
  m.validate(c,h)
  for field,value in [('network','mainnet'),('role','observer'),('node_id','v2')]:
   with self.assertRaises(ValueError):m.validate(dict(c,**{field:value}),h)
  with self.assertRaises(ValueError):m.validate(c,dict(h,chain_id=4735489))
 def test_separate_key_and_explicit_chain(self):
  text=m.unit(Path('/opt/hashburst-apow-miner/test/hvm-apow-miner'))
  self.assertIn('--chain-id 4735490 --loop',text)
  self.assertIn('User=hashburst-apow-miner',text)
  self.assertIn('/var/lib/hashburst-apow-miner/miner.key',text)
  self.assertNotIn('consensus.key',text)
  self.assertIn('CPUQuota=25%',text)
if __name__=='__main__':unittest.main()
