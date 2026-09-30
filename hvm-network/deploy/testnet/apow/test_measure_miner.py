"""Integration with the built Go miner; no real node or wallet is used."""
import importlib.util,json,os,subprocess,tempfile,threading,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
s=importlib.util.spec_from_file_location('measure_node',R/'measure-node.py');m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class RealMinerIntegration(unittest.TestCase):
 def test_go_miner_signed_work_against_isolated_endpoint(self):
  binary=os.environ.get('HVM_MINER_TEST_BINARY')
  if not binary:self.skipTest('HVM_MINER_TEST_BINARY required; no live miner test claimed')
  with tempfile.TemporaryDirectory() as temp:
   key=Path(temp)/'miner.key'
   created=subprocess.check_output([binary,'--key-file',str(key),'--generate-key'],text=True,timeout=15)
   identity=subprocess.check_output([binary,'--key-file',str(key),'--identity-only'],text=True,timeout=15)
   address=identity.strip().split('=',1)[1]
   job={'chain_id':4735490,'height':1,'parent_hash':'ab'*32,'poh':0,'bits':4,'epoch_start':0,'nonce':0}
   server=m.ChallengeServer(job,address);t=threading.Thread(target=server.serve_forever);t.start()
   try:
    cmd=[binary,'--key-file',str(key),'--chain-id','4735490','--endpoint','http://127.0.0.1:'+str(server.server_port)+server.token,'--timeout','5s']
    result=subprocess.run(cmd,capture_output=True,text=True,timeout=15)
    self.assertEqual(result.returncode,0,result.stderr);self.assertIn('APOW_SUBMITTED',result.stdout)
    self.assertIsNotNone(server.proof);self.assertIsNone(server.failure)
    m.check_proof(server.proof,job,address)
   finally:server.shutdown();server.server_close();t.join()
if __name__=='__main__':unittest.main()
