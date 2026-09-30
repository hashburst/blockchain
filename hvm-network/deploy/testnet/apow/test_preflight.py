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

class SerializationGates(unittest.TestCase):
 def proofs(self):
  row={k:'abc' for k in p.FIELDS}
  row.update(chain_id=4735490,height=7,evm_gas_used=0,certificate={'present':True})
  return [copy.deepcopy(row) for _ in range(5)]
 def test_omitted_zero_and_no_evidence_mutation(self):
  rows=self.proofs();del rows[2]['evm_gas_used'];original=copy.deepcopy(rows)
  p.compare(rows,7);self.assertEqual(rows,original)
 def test_omitted_zero_does_not_hide_nonzero(self):
  rows=self.proofs();del rows[0]['evm_gas_used'];rows[1]['evm_gas_used']=1
  with self.assertRaisesRegex(ValueError,'evm_gas_used'):p.compare(rows,7)
 def test_invalid_gas(self):
  for value in (None,True,-1,'0',0.0,2**64):
   with self.subTest(value=value):
    rows=self.proofs();rows[0]['evm_gas_used']=value
    with self.assertRaisesRegex(ValueError,'invalid evm_gas_used'):p.compare(rows,7)
 def test_roots_remain_required(self):
  for field in p.FIELDS:
   if field in ('chain_id','height','evm_gas_used'):continue
   for value in (None,''):
    rows=self.proofs();rows[0][field]=value
    with self.assertRaisesRegex(ValueError,field):p.compare(rows,7)
   rows=self.proofs();del rows[0][field]
   with self.assertRaisesRegex(ValueError,field):p.compare(rows,7)
 def test_offline_incomplete_and_complete_progress(self):
  import tempfile,json
  from pathlib import Path
  from unittest.mock import patch
  rows=[{'node_id':n,'digest':'same','protocol':{},'genesis':'same','peer_id':str(i),
         'height':7,'pin_sha256':str(i),'service':'ActiveState=active\nSubState=running'} for i,(_,n) in enumerate(p.TARGETS)]
  with tempfile.TemporaryDirectory() as tmp, patch.object(p.transport,'Session',side_effect=AssertionError('SSH forbidden')):
   d=Path(tmp);(d/'nodes.json').write_text(json.dumps(rows));(d/'commitments.json').write_text(json.dumps(self.proofs()))
   result=p.verify_report(d);self.assertFalse(result['saved_progress_verified']);self.assertFalse(result['activation_authorized'])
   self.assertFalse((d/'READY.json').exists())
   later=copy.deepcopy(rows)
   for r in later:r['height']=8
   (d/'progress.json').write_text(json.dumps(list(reversed(later))))
   self.assertTrue(p.verify_report(d)['saved_progress_verified'])
   later[0]['pin_sha256']='changed';(d/'progress.json').write_text(json.dumps(later))
   with self.assertRaisesRegex(ValueError,'configuration changed'):p.verify_report(d)

if __name__=='__main__':unittest.main()
