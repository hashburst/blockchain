import importlib.util, tempfile, unittest, json, fcntl
from pathlib import Path
spec=importlib.util.spec_from_file_location('installer',Path(__file__).with_name('hashburst-install.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class InstallerTests(unittest.TestCase):
 def test_atomic_record_and_prefix_recovery(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d);cfg=p/'node.json';cfg.write_text('{}');(p/'runtime.pin').write_text('identity')
   for n in ('consensus-votes.jsonl','consensus-bft-signatures.jsonl'):(p/n).write_text('signed\n')
   r={'phase':'stopped','config':m.digest(cfg),'pin':m.digest(p/'runtime.pin'),'journals':m.prefix(p)}
   m.atomic(p/'state.json',r);self.assertEqual(json.loads((p/'state.json').read_text()),r)
   with (p/'consensus-votes.jsonl').open('a') as f:f.write('later\n')
   m.preserved(r,cfg,p)
   (p/'consensus-votes.jsonl').write_text('changed\n')
   with self.assertRaises(RuntimeError):m.preserved(r,cfg,p)
 def test_mainnet_manifest_fails_closed(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/'release.json';p.write_text(json.dumps({'chain_id':4735489,'sha256':'a'*64,'source_commit':'b'*40,'predecessors':['c'*64]}))
   with self.assertRaises(RuntimeError):m.manifest(p)
 def test_effective_command_exact(self):
  self.assertTrue(m.effective({'ExecStart':'{ path=/bin/node ; argv[]=/bin/node --config /etc/node.json ; }'},'/bin/node','/etc/node.json'))
  self.assertFalse(m.effective({'ExecStart':'{ path=/bin/old ; argv[]=/bin/old --config /etc/node.json ; }'},'/bin/node','/etc/node.json'))
 def test_cross_release_node_lock(self):
  with tempfile.TemporaryDirectory() as d:
   node=Path(d)/'node';a=node/'release-a';b=node/'release-b';a.mkdir(parents=True);b.mkdir()
   with (a.parent/'worker.lock').open('a') as first, (b.parent/'worker.lock').open('a') as second:
    fcntl.flock(first,fcntl.LOCK_EX|fcntl.LOCK_NB)
    with self.assertRaises(BlockingIOError):fcntl.flock(second,fcntl.LOCK_EX|fcntl.LOCK_NB)
 def test_prefix_truncation(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/'journal';p.write_bytes(b'a')
   with self.assertRaises(RuntimeError):m.digest(p,2)
class AsyncStartTests(unittest.TestCase):
 def properties(self, active='active'):
  return {'ExecStart':'{ path=/bin/node ; argv[]=/bin/node --config /etc/node.json ; }',
          'ActiveState':active,'SubState':'running' if active=='active' else 'start','MainPID':'42'}
 def test_activation_helper_not_treated_as_wrong_binary(self):
  from unittest.mock import patch, MagicMock
  import io
  opener=MagicMock()
  opener.open.side_effect=[io.StringIO(json.dumps(dict(chain_id=4735490,node_id='n',peer_id='p',role='observer',reactor_running=True,peer_count=4,finalized_height=h))) for h in (10,11)]
  with patch.object(m,'props',side_effect=[self.properties('activating')]+[self.properties()]*4), patch.object(m,'digest',return_value='expected') as digest, patch.object(m,'preserved') as preserved, patch.object(m.time,'sleep'), patch.object(m.urllib.request,'build_opener',return_value=opener):
   result=m.verify('unit','/etc/node.json',dict(node_id='n',peer_id='p',role='observer',data_dir='/data'),'/bin/node',{'sha256':'expected'},60)
   self.assertEqual(result['finalized_height'],11)
   self.assertEqual(digest.call_count,2)
   preserved.assert_called_once()
 def test_stable_wrong_executable_still_rejected(self):
  from unittest.mock import patch
  with patch.object(m,'props',return_value=self.properties()), patch.object(m,'digest',return_value='wrong'):
   with self.assertRaisesRegex(RuntimeError,'running binary differs'):
    m.verify('unit','/etc/node.json',{},'/bin/node',{'sha256':'expected'},60)
 def test_systemd_error_is_visible(self):
  from unittest.mock import patch
  import subprocess
  with patch.object(m.subprocess,'run',side_effect=subprocess.CalledProcessError(1,['systemd-run'],stderr='Unit collision')):
   with self.assertRaisesRegex(RuntimeError,'Unit collision'):m.run('systemd-run')
 def test_nonblocking_job_request(self):
  import ast
  tree=ast.parse(Path(m.__file__).read_text())
  calls=[n for n in ast.walk(tree) if isinstance(n,ast.Call) and isinstance(n.func,ast.Name) and n.func.id=='run' and n.args and isinstance(n.args[0],ast.Constant) and n.args[0].value=='systemd-run']
  self.assertEqual(len(calls),1)
  self.assertIn('--no-block',[a.value for a in calls[0].args if isinstance(a,ast.Constant)])
if __name__=='__main__':unittest.main()
