import importlib.util,unittest
from pathlib import Path
from unittest.mock import patch
s=importlib.util.spec_from_file_location('n',Path(__file__).with_name('node-api.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Gates(unittest.TestCase):
 def test_health(self):
  d=dict(chain_id=4735490,config_digest=m.DIGEST,node_id='v1',role='validator',reactor_running=True,peer_count=3,finalized_height=100)
  with patch.object(m,'get',return_value=d):self.assertEqual(m.health({'node_id':'v1'}),d)
  for key,value in [('chain_id',1337),('role','observer'),('config_digest','bad'),('peer_count',2),('reactor_running',False),('finalized_height',7)]:
   with patch.object(m,'get',return_value=dict(d,**{key:value})),self.assertRaises(RuntimeError):m.health({'node_id':'v1'})
 def test_prefix(self):
  import tempfile
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/'journal';p.write_bytes(b'original\n')
   digest=m.prefix(p,9);p.write_bytes(b'original\nnew\n');self.assertEqual(digest,m.prefix(p,9))
   p.write_bytes(b'bad')
   with self.assertRaises(RuntimeError):m.prefix(p,9)

class ResumeSafety(unittest.TestCase):
 def test_existing_override_never_stops_or_starts_service(self):
  import tempfile,base64,hashlib
  with tempfile.TemporaryDirectory() as tmp:
   fake=Path(tmp);override=fake/'override';override.write_text('exists')
   release=fake/'release';release.mkdir();(release/'hashburst-testnet').write_bytes(b'binary')
   real=m.Path
   def paths(s):
    if s.startswith('/opt/'):return release
    if s.endswith('/30-api-runtime.conf'):return override
    return real(s)
   with patch.object(m,'Path',side_effect=paths),patch.object(m.os,'geteuid',return_value=0),patch.object(m.socket,'gethostname',return_value='node'),patch.object(m,'health',return_value={}),patch.object(m,'recovered',return_value={'no_restart':True}) as recovery,patch.object(m,'run') as run:
    result=m.main(dict(node={'hostname':'node'},action='upgrade',binary=base64.b64encode(b'binary').decode(),sha256=hashlib.sha256(b'binary').hexdigest()))
    self.assertTrue(result['no_restart']);recovery.assert_called_once();run.assert_not_called()

if __name__=='__main__':unittest.main()
