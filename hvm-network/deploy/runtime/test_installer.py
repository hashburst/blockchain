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
class ReadinessTests(unittest.TestCase):
 def setUp(self):
  import subprocess
  spec=importlib.util.spec_from_file_location('rollout',Path(__file__).with_name('resume-rollout.py'))
  module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
  self.ns={'subprocess':subprocess};exec(module.READ_ONLY_WAIT,self.ns)
 def test_read_recovers_without_restart(self):
  from unittest.mock import Mock, patch
  import subprocess, urllib.error
  call=Mock(side_effect=[urllib.error.URLError(ConnectionRefusedError(111,'refused')),{'height':5}])
  with patch.object(subprocess,'run',return_value=Mock(stdout='ActiveState=active\nSubState=running\n')) as run, patch.object(self.ns['time'],'sleep'):
   self.assertEqual(self.ns['read_only_ready'](call,{'action':'status'}),{'height':5})
   self.assertEqual(run.call_args.args[0][:2],['systemctl','show'])
   self.assertEqual(run.call_count,1)
 def test_identity_error_not_retried(self):
  from unittest.mock import Mock
  call=Mock(side_effect=RuntimeError('identity mismatch'))
  with self.assertRaisesRegex(RuntimeError,'identity mismatch'):self.ns['read_only_ready'](call,{'action':'status'})
  self.assertEqual(call.call_count,1)
 def test_write_action_rejected(self):
  from unittest.mock import Mock
  call=Mock()
  with self.assertRaises(RuntimeError):self.ns['read_only_ready'](call,{'action':'install'})
  call.assert_not_called()
 def test_failed_service_shows_journal_and_stops(self):
  from unittest.mock import Mock, patch
  import subprocess, urllib.error
  call=Mock(side_effect=urllib.error.URLError(ConnectionRefusedError(111,'refused')))
  with patch.object(subprocess,'run',side_effect=[Mock(stdout='ActiveState=failed\n'),Mock(stdout='failure details')]) as run:
   with self.assertRaisesRegex(RuntimeError,'no restart performed'):self.ns['read_only_ready'](call,{'action':'status'})
   self.assertEqual(run.call_args_list[1].args[0][0],'journalctl')
   self.assertEqual(call.call_count,1)
if __name__=='__main__':unittest.main()
