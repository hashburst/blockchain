import importlib.util,json,unittest
from pathlib import Path
from unittest.mock import patch
R=Path(__file__).resolve().parent

def load(name):
 spec=importlib.util.spec_from_file_location(name,R/(name+'.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
installer=load('hashburst-install');recovery=load('recover-v1')

class RecoveryTests(unittest.TestCase):
 def setUp(self):
  self.release=json.loads((R/'v1-recovery-release.json').read_text())
  self.p={'MainPID':'0','ActiveState':'failed','Result':'exit-code','ExecMainStatus':'1'}
 def test_failed_target_explicitly_allowed(self):
  installer.predecessor_allowed(self.p,recovery.OLD_SHA,self.release,recovery.NODE)
 def test_other_node_rejected(self):
  with self.assertRaises(RuntimeError):installer.predecessor_allowed(self.p,recovery.OLD_SHA,self.release,'hvm-testnet-v2')
 def test_live_target_rejected(self):
  self.p.update(MainPID='99',ActiveState='active')
  with self.assertRaises(RuntimeError):installer.predecessor_allowed(self.p,recovery.OLD_SHA,self.release,recovery.NODE)
 def test_other_failure_rejected(self):
  self.p['Result']='oom-kill'
  with self.assertRaises(RuntimeError):installer.predecessor_allowed(self.p,recovery.OLD_SHA,self.release,recovery.NODE)
 def test_unknown_predecessor_rejected(self):
  with self.assertRaises(RuntimeError):installer.predecessor_allowed(self.p,'0'*64,self.release,recovery.NODE)
 def test_standard_install_still_requires_running(self):
  self.release.pop('failed_node_recovery')
  with self.assertRaises(RuntimeError):installer.predecessor_allowed(self.p,recovery.OLD_SHA,self.release,recovery.NODE)
 def test_manifest_binds_installer(self):
  self.assertEqual(recovery.sha(R/'hashburst-install.py'),self.release['installer_sha256'])
 def test_redirect_drops_token(self):
  from urllib.request import Request
  req=Request('https://api.github.com/artifact',headers={'Authorization':'Bearer secret'})
  out=recovery.SafeRedirect().redirect_request(req,None,302,'',{},'https://download.example/file')
  self.assertIsNone(out.get_header('Authorization'))
  with self.assertRaises(RuntimeError):recovery.SafeRedirect().redirect_request(req,None,302,'',{},'http://download.example/file')
 def test_commitment_disagreement_rejected(self):
  p={'chain_id':4735490,'height':10,'certificate':{'present':True},'hash':'a'}
  recovery.compare_proofs([dict(p) for _ in range(4)],10,('chain_id','height','hash'))
  bad=dict(p,hash='b')
  with self.assertRaises(RuntimeError):recovery.compare_proofs([p,p,p,bad],10,('chain_id','height','hash'))
 def test_missing_peer_rejected(self):
  with self.assertRaises(RuntimeError):recovery.compare_rows([],recovery.PEERS)
 def test_remote_programs_compile(self):
  for code in (recovery.INSPECT,recovery.REQUEST,recovery.EXISTING,recovery.WAIT):compile(code,'remote','exec')

if __name__=='__main__':unittest.main()
