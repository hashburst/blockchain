import importlib.util,tempfile,unittest
from pathlib import Path
s=importlib.util.spec_from_file_location('repair',Path(__file__).with_name('repair-node.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Repair(unittest.TestCase):
 def test_merged_override_checked(self):
  self.assertTrue(m.effective('{ path=/new ; argv[]=/new --config /cfg ; }',Path('/new'),Path('/cfg')))
  self.assertFalse(m.effective('{ path=/old ; argv[]=/old --config /cfg ; }',Path('/new'),Path('/cfg')))
  self.assertFalse(m.effective('{ path=/new ; argv[]=/new --config /wrong ; }',Path('/new'),Path('/cfg')))
 def test_prefix_append_allowed_rewrite_rejected(self):
  with tempfile.TemporaryDirectory() as d:
   d=Path(d);cfg=d/'config';cfg.write_text('config');(d/'runtime.pin').write_text('identity')
   for f in ('consensus-votes.jsonl','consensus-bft-signatures.jsonl'):(d/f).write_bytes(b'original')
   r={'config_sha256':m.digest(cfg),'pin_sha256':m.digest(d/'runtime.pin'),'journals':m.prefix_proof(d)}
   (d/'consensus-votes.jsonl').write_bytes(b'original appended');m.check_preserved(r,cfg,d)
   (d/'consensus-votes.jsonl').write_bytes(b'changed')
   with self.assertRaises(RuntimeError):m.check_preserved(r,cfg,d)
 def test_truncation_fails(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/'j';p.write_bytes(b'a')
   with self.assertRaises(RuntimeError):m.digest(p,2)
if __name__=='__main__':unittest.main()
