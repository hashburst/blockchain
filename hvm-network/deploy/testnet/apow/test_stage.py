import hashlib,importlib.util,json,tempfile,unittest,tarfile,io
from pathlib import Path
from unittest.mock import patch
R=Path(__file__).resolve().parent
def load(name,file):
 s=importlib.util.spec_from_file_location(name,R/file);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
node=load('stage_node','stage-node.py');client=load('stage_all','stage-all.py')
class StagingGuards(unittest.TestCase):
 def test_journal_append_allowed_but_prefix_change_rejected(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'journal';p.write_bytes(b'original\n');proof={str(p):{'size':9,'sha256':node.digest(p),'exact':False}}
   p.write_bytes(b'original\nnew\n');node.verify_protected(proof)
   p.write_bytes(b'different\n')
   with self.assertRaises(ValueError):node.verify_protected(proof)
 def test_pin_append_rejected(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'pin';p.write_bytes(b'pin');proof={str(p):{'size':3,'sha256':node.digest(p),'exact':True}}
   p.write_bytes(b'pin-new')
   with self.assertRaises(ValueError):node.verify_protected(proof)
 def test_short_prefix_rejected(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'journal';p.write_bytes(b'a')
   with self.assertRaises(ValueError):node.digest(p,2)
 def archive(self,p,symlink=False,duplicate=False):
  with tarfile.open(p,'w:gz') as tar:
   for name in ('hashburst-testnet','hvm-apow-miner'):
    m=tarfile.TarInfo(client.ROOT+'/'+name);m.size=3
    if symlink:m.type=tarfile.SYMTYPE;m.linkname='/etc/passwd';m.size=0
    tar.addfile(m,io.BytesIO(b'abc'))
    if duplicate:tar.addfile(m,io.BytesIO(b'abc'))
 def test_wrong_release_refused(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'release.tar.gz';self.archive(p)
   with self.assertRaises(ValueError):client.unpack_release(p,Path(t))
 def test_extracts_only_regular_named_binaries(self):
  for bad in ('symlink','duplicate',None):
   with self.subTest(case=bad),tempfile.TemporaryDirectory() as t:
    p=Path(t)/'release.tar.gz';self.archive(p,symlink=bad=='symlink',duplicate=bad=='duplicate')
    with patch.object(client,'ARCHIVE_SHA256',hashlib.sha256(p.read_bytes()).hexdigest()):
     if bad:
      with self.assertRaises(ValueError):client.unpack_release(p,Path(t))
     else:
      client.unpack_release(p,Path(t));self.assertEqual((Path(t)/'hashburst-testnet').read_bytes(),b'abc')
 def test_wrong_host_refused_before_state_read(self):
  with patch.object(node.os,'geteuid',return_value=0),patch.object(node.platform,'machine',return_value='x86_64'),patch.object(node.subprocess,'check_output',return_value='inet 192.0.2.1/24'):
   with self.assertRaisesRegex(ValueError,'wrong host'):node.stage({'node_id':'hvm-testnet-v1'})
 def test_remote_combined_source_compiles(self):
  compile(client.pre.SOURCE+'\ninspect=main\n'+(R/'stage-node.py').read_text(),'remote','exec')
if __name__=='__main__':unittest.main()
