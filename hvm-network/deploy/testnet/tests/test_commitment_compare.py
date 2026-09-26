import importlib.util,unittest
from pathlib import Path
s=importlib.util.spec_from_file_location('compare',Path(__file__).parents[1]/'compare-finalized.py')
m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Compare(unittest.TestCase):
 def setUp(self):
  self.row=dict.fromkeys(m.FIELDS,'abc');self.row.update(chain_id=4735490,height=10,certificate={'signatures':['a']})
 def test_different_valid_certificate_subsets_not_compared(self):
  rows=[dict(self.row) for _ in range(4)];rows[-1]['certificate']={'signatures':['b']}
  m.compare(rows,10)
 def test_reject_different_commitment_height_chain_or_missing_certificate(self):
  for field,value in [('hash','other'),('height',11),('chain_id',1337),('certificate',None),('hvm_state_root','')]:
   rows=[dict(self.row) for _ in range(4)];rows[-1][field]=value
   with self.subTest(field=field),self.assertRaises(ValueError):m.compare(rows,10)
if __name__=='__main__':unittest.main()
