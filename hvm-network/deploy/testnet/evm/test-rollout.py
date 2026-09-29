import importlib.util,unittest
from pathlib import Path
s=importlib.util.spec_from_file_location('rollout',Path(__file__).with_name('rollout.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Gates(unittest.TestCase):
 def test_common_height(self):
  row=dict(chain_id=4735490,height=100,hash='same',certificate={'signers':[]},evm_state_root='root',evm_receipts_root='receipts')
  m.compare([row.copy() for _ in range(5)],100)
  for field in ('hash','evm_state_root','validator_set_root'):
   other=dict(row);other[field]='different'
   with self.assertRaises(RuntimeError):m.compare([row,other],100)
 def test_height_and_legacy(self):
  for row in ({'chain_id':1337,'height':100},{'chain_id':4735490,'height':99}):
   with self.assertRaises(RuntimeError):m.compare([row],100)
if __name__=='__main__':unittest.main()
