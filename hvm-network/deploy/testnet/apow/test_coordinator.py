import copy,importlib.util,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('coordinator',R/'coordinator.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
g=m.ns
class Guards(unittest.TestCase):
 def plan(self):return {'schema':1,'chain_id':4735490,'activation':dict(g['PROFILE'],activation_height=5000),'binary_sha256':g['BIN_SHA'],'nodes':[{'node_id':n,'height':1000} for _,n in m.pre.TARGETS],'common_height':1000,'commitments':[]}
 def test_missing_job_is_not_a_command_failure(self):
  import subprocess
  reply=subprocess.CompletedProcess([],1,'LoadState=not-found\nActiveState=inactive\nMainPID=0\n','')
  with patch('subprocess.run',return_value=reply):self.assertEqual(g['properties']('missing')['LoadState'],'not-found')
 def test_service_query_error_is_not_missing_job(self):
  import subprocess
  reply=subprocess.CompletedProcess([],1,'','bus unavailable')
  with patch('subprocess.run',return_value=reply):
   with self.assertRaises(RuntimeError):g['properties']('unit')
 def test_exact_binary_execution_exception_only(self):
  text=g['start_text'](Path('/etc/example/node.json'))
  self.assertIn('ExecPaths='+str(g['BIN']),text)
  self.assertNotIn('NoExecPaths=',text)
  self.assertNotIn('ProtectSystem=',text)
 def test_later_execstart_override_rejected(self):
  with patch.dict(g,command=lambda *a:'{ path=/old/node ; argv[]=/old/node --config /etc/example/node.json ; }'):
   with self.assertRaises(ValueError):g['check_effective_start']('unit',Path('/etc/example/node.json'))
 def test_effective_execstart_matches(self):
  cfg=Path('/etc/example/node.json');b=str(g['BIN'])
  with patch.dict(g,command=lambda *a:'{ path='+b+' ; argv[]='+b+' --config '+str(cfg)+' ; }'):
   self.assertTrue(g['check_effective_start']('unit',cfg))
 def test_valid_plan(self):self.assertEqual(len(g['validate_plan'](self.plan())),64)
 def test_reject_other_chain(self):
  p=self.plan();p['chain_id']=4735489
  with self.assertRaises(ValueError):g['validate_plan'](p)
 def test_reject_gas_change(self):
  p=self.plan();p['activation']['gas_limit']=2000000
  with self.assertRaises(ValueError):g['validate_plan'](p)
 def test_reject_insufficient_margin(self):
  p=self.plan();p['activation']['activation_height']=2000
  with self.assertRaises(ValueError):g['validate_plan'](p)
 def test_reject_missing_node(self):
  p=self.plan();p['nodes'].pop()
  with self.assertRaises(ValueError):g['validate_plan'](p)
 def test_plan_id_binds_profile(self):
  p=self.plan();q=copy.deepcopy(p);q['activation']['activation_height']+=1
  self.assertNotEqual(g['validate_plan'](p),g['validate_plan'](q))
 def test_stopped_refuses_running_pid(self):
  with patch.dict(g,properties=lambda u:{'ActiveState':'inactive','MainPID':'42'}):
   with self.assertRaises(ValueError):g['stopped']('unit')
 def test_stopped_accepts_failed_no_pid(self):
  with patch.dict(g,properties=lambda u:{'ActiveState':'failed','MainPID':'0'}):g['stopped']('unit')
 def test_records_cannot_be_overwritten(self):
  with tempfile.TemporaryDirectory() as t:
   p=Path(t)/'record';g['save'](p,{'a':1});g['save'](p,{'a':1})
   with self.assertRaises(ValueError):g['save'](p,{'a':2})
 def test_systemd_job_launch_never_waits_on_ssh(self):
  source=(R/'migrate-node.py').read_text()
  self.assertIn("command('systemd-run','--no-block',",source)
  self.assertIn("'--property=TimeoutStartSec=infinity'",source)
 def test_worker_survives_transport_source_compile(self):compile(m.worker_source,'worker','exec')
 def test_worker_failure_does_not_write_done(self):
  with tempfile.TemporaryDirectory() as t:
   d=Path(t);g['save'](d/'record.json',{'node_id':'hvm-testnet-v1','plan':'x','service_user':'runtime'})
   with patch.dict(g,stopped=lambda u:(_ for _ in ()).throw(ValueError('running'))):
    with self.assertRaises(ValueError):g['worker'](d)
   self.assertTrue((d/'FAILED.json').exists());self.assertFalse((d/'DONE.json').exists())
 def test_offline_gate_refuses_failed_record(self):
  with tempfile.TemporaryDirectory() as t:
   d=Path(t);g['save'](d/'DONE.json',{'ok':False,'plan':'x'})
   with patch.dict(g,stopped=lambda u:None):
    with self.assertRaises(ValueError):g['local_gate'](d,d/'cfg','unit',self.plan())
 def test_snapshot_detects_journal_mutation(self):
  with tempfile.TemporaryDirectory() as t:
   d=Path(t);cfg={'data_dir':t}
   for f in ('blockchain.dat','blockchain.idx','consensus-votes.jsonl','consensus-bft-signatures.jsonl'):(d/f).write_bytes(b'initial')
   old=g['fingerprints'](cfg);(d/'consensus-votes.jsonl').write_bytes(b'changed')
   with self.assertRaises(ValueError):g['check_snapshot'](cfg,old)
 def test_guard_denies_unapproved_start(self):
  self.assertIn('ExecStartPre=/usr/bin/test -f /example/START_AUTHORIZED',g['guard_text'](Path('/example')))
if __name__=='__main__':unittest.main()
