#!/usr/bin/env python3
import importlib.util,json,tempfile,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('proposal',R/'propose-parameters.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class MainnetProposal(unittest.TestCase):
 def fixture(self):
  return {'nodes':[{'preflight_ok':True,'apow_hashes_per_second':4096,'evm_safe_gas_limit':300000} for _ in range(4)]+[{'preflight_ok':True,'apow_hashes_per_second':1,'evm_safe_gas_limit':250000}]}
 def test_bounded(self):
  p=m.proposal(self.fixture());self.assertEqual(p['chain_id'],4735489);self.assertEqual(p['apow']['initial_bits'],12);self.assertEqual(p['evm']['gas_limit'],250000);self.assertIn('NOT_ACTIVATED',p['status'])
 def test_requires_five(self):
  d=self.fixture();d['nodes'].pop()
  with self.assertRaises(ValueError):m.proposal(d)
 def test_rejects_failed_preflight(self):
  d=self.fixture();d['nodes'][2]['preflight_ok']=False
  with self.assertRaises(ValueError):m.proposal(d)
 def test_rejects_bad_measurement(self):
  d=self.fixture();d['nodes'][0]['apow_hashes_per_second']=0
  with self.assertRaises(ValueError):m.proposal(d)
if __name__=='__main__':unittest.main()
