import importlib.util,json,tempfile,unittest
from pathlib import Path
from unittest.mock import Mock,patch
R=Path(__file__).resolve().parent

def load(name):
 s=importlib.util.spec_from_file_location(name,R/(name+'.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
f=load('align-fleet');i=load('fleet-install')
class FleetTests(unittest.TestCase):
 def test_override_replacement_backup_and_idempotence(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'zzzz-hvm-runtime.conf'
   old='[Service]\nExecStart=\nExecStart=/old --config /cfg\nExecPaths=/old\n';p.write_text(old)
   new=old.replace('/old','/new');i.replace_override(p,new,'/old','/cfg')
   self.assertEqual(p.read_text(),new)
   self.assertEqual(list(p.parent.glob('*.previous-*'))[0].read_text(),old)
   i.replace_override(p,new,'/old','/cfg')
 def test_unknown_override_and_symlink_rejected(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'override';p.write_text('custom')
   with self.assertRaises(RuntimeError):i.replace_override(p,'new','/old','/cfg')
   self.assertEqual(p.read_text(),'custom');q=Path(t)/'link';q.symlink_to(p)
   with self.assertRaises(RuntimeError):i.replace_override(q,'new','/old','/cfg')
 def test_manifest_preserves_runtime_and_old_recovery(self):
  r=json.loads((R/'fleet-release.json').read_text());self.assertEqual(r['sha256'],f.r.BINARY_SHA)
  self.assertEqual(r['installer_sha256'],f.r.sha(R/'fleet-install.py'))
  self.assertNotIn('failed_node_recovery',r)
  old=json.loads((R/'v1-recovery-release.json').read_text())
  self.assertEqual(old['installer_sha256'],f.r.sha(R/'hashburst-install.py'))
 def test_binary_gate_refuses_unexpected_and_old_completed(self):
  rows=[{'node_id':f.r.NODE,'binary_sha256':f.r.BINARY_SHA},{'node_id':'hvm-testnet-v2','binary_sha256':f.OLD}]
  f.check_binaries(rows,[])
  with self.assertRaises(RuntimeError):f.check_binaries(rows,['hvm-testnet-v2'])
 def test_resume_only_verifies_after_start(self):
  d=Mock();d.remote.side_effect=[{'phase':'start_requested','active':'inactive'},{}]
  f.wait(d,'host','node',10)
  self.assertEqual(d.remote.call_args.args[1],f.VERIFY)
 def test_stopped_before_start_never_restarts(self):
  d=Mock();d.remote.return_value={'phase':'stopped','active':'failed'}
  with self.assertRaises(RuntimeError):f.wait(d,'host','node',10)
  self.assertEqual(d.remote.call_count,1)
 def test_remote_programs_compile(self):
  for s in (f.STATE,f.REQUEST,f.VERIFY):compile(s,'remote','exec')
if __name__=='__main__':unittest.main()
