import hashlib,os,platform,shutil,subprocess,tempfile,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
class Installer(unittest.TestCase):
 def fixture(self,d):
  src=Path(d)/'package';src.mkdir()
  shutil.copy(R/'install-tools.sh',src/'install-tools.sh')
  files=['hashburst-wallet','hashburst-testnet','hashburst-mainnet','hvm-apow-miner','prompt.py','WALLET.md','RELEASE-GATES.md','economics.draft.json','STATUS.txt','RELEASE_NOTES.md']
  for f in files:(src/f).write_text('test fixture, not executable\n')
  (src/'SOURCE_COMMIT').write_text('a'*40+'\n')
  arch={'x86_64':'amd64','aarch64':'arm64','arm64':'arm64'}[platform.machine()]
  (src/'TARGET').write_text(platform.system().lower()+'-'+arch+'\n')
  manifest=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(src.iterdir()))
  (src/'SHA256SUMS').write_text(manifest)
  env=dict(os.environ,HASHBURST_TOOLS_DIR=str(Path(d)/'tools'))
  return src,env
 def test_install_exclusive_and_architecture_bound(self):
  with tempfile.TemporaryDirectory() as d:
   src,env=self.fixture(d)
   result=subprocess.run(['bash',str(src/'install-tools.sh')],env=env,capture_output=True,text=True)
   self.assertEqual(result.returncode,0,result.stderr)
   self.assertIn('NO_SERVICE_STARTED_NO_NETWORK_ACTIVATED',result.stdout)
   result=subprocess.run(['bash',str(src/'install-tools.sh')],env=env,capture_output=True,text=True)
   self.assertNotEqual(result.returncode,0)
 def test_tampering_aborts_before_install(self):
  with tempfile.TemporaryDirectory() as d:
   src,env=self.fixture(d);(src/'hashburst-wallet').write_text('tampered')
   result=subprocess.run(['bash',str(src/'install-tools.sh')],env=env,capture_output=True,text=True)
   self.assertNotEqual(result.returncode,0)
   self.assertFalse(Path(env['HASHBURST_TOOLS_DIR']).exists())
if __name__=='__main__':unittest.main()
