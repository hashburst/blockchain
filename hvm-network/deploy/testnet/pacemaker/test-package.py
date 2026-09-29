import importlib.util,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
def load(name):
 s=importlib.util.spec_from_file_location(name,R/(name+'.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
roll=load('rollout');dash=load('install-dashboard')
class Guards(unittest.TestCase):
 def test_stall(self):
  p=roll.Progress();p.update(51473,0);p.update(51473,599)
  with self.assertRaisesRegex(RuntimeError,'STALLED'):p.update(51473,600)
 def test_progress(self):
  p=roll.Progress();p.update(1,0);p.update(2,599);p.update(2,1000)
  with self.assertRaisesRegex(RuntimeError,'regressed'):p.update(1,1001)
 def test_agreement(self):
  p={'chain_id':4735490,'height':51473,'hash':'a','certificate':{'present':True}}
  roll.compare([p.copy() for _ in range(5)],51473)
  with self.assertRaisesRegex(RuntimeError,'disagree'):roll.compare([p,{**p,'hash':'b'}],51473)
  with self.assertRaisesRegex(RuntimeError,'missing EVM'):roll.compare([{**p,'height':53303}],53303)
 def test_dashboard_patch(self):
  raw='    <button data-view="storage" id="nav-storage">Sovereign Storage</button>\n</main>\n<script nonce="kept">\nloadView(\'explorer\');\n</script>'
  out=dash.patch(raw);self.assertIn('nonce="kept"',out);self.assertEqual(dash.patch(out),out);self.assertEqual(out.count('id="nav-hvm"'),1)
  with self.assertRaises(RuntimeError):dash.patch('unrecognized source')
 def test_no_migration_or_state_overwrite(self):
  s=(R/'node.py').read_text();self.assertNotIn('--migrate-evm',s);self.assertNotIn('unlink(',s);self.assertNotIn('rmtree(',s)
if __name__=='__main__':unittest.main()
