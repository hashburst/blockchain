import copy,unittest
import preflight as p
class Gates(unittest.TestCase):
 def test_common_height(self):
  proof={k:'abc' for k in p.FIELDS};proof.update(chain_id=4735490,height=7,evm_gas_used=0,certificate={'present':True})
  rows=[copy.deepcopy(proof) for _ in range(5)];p.compare(rows,7)
  for k in p.FIELDS:
   bad=copy.deepcopy(rows);bad[4][k]='different'
   with self.assertRaises(ValueError):p.compare(bad,7)
  rows[0]['certificate']=None
  with self.assertRaises(ValueError):p.compare(rows,7)
 def test_no_mutating_operations(self):
  for command in ('systemctl stop','systemctl restart','write_text','migrate-apow'):
   self.assertNotIn(command,p.SOURCE)
 def test_duplicate_peer(self):
  rows=[{'node_id':n,'digest':'same','protocol':{},'genesis':'same','peer_id':str(i),'service':'ActiveState=active\nSubState=running'} for i,(_,n) in enumerate(p.TARGETS)]
  p.validate_rows(rows);rows[-1]['peer_id']=rows[0]['peer_id']
  with self.assertRaises(ValueError):p.validate_rows(rows)
if __name__=='__main__':unittest.main()
