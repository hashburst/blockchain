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
if __name__=='__main__':unittest.main()
