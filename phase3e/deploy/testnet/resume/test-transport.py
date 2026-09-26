import concurrent.futures,importlib.util,sys,unittest
from pathlib import Path
s=importlib.util.spec_from_file_location('ssh_session',Path(__file__).with_name('ssh-session.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
SOURCE='''counter=0
def main(payload):
 global counter
 counter+=1
 if payload.get('fail'):raise RuntimeError('expected remote rejection')
 print('diagnostic output')
 return {'counter':counter,'node':payload['node']}
'''
class Transport(unittest.TestCase):
 def test_no_multiplexing(self):
  c=m.ssh_command('77.90.188.153')
  for option in ['ControlMaster=no','ControlPath=none','ControlPersist=no','BatchMode=no']:self.assertIn(option,c)
  self.assertNotIn('-MNf',c)
 def test_four_persistent_sessions(self):
  sessions=[]
  try:
   for _ in range(4):sessions.append(m.Session([sys.executable,'-u','-c',m.RUNNER],SOURCE))
   for count in (1,2):
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
     result=list(pool.map(lambda pair:pair[1].call({'node':pair[0]}),enumerate(sessions)))
    self.assertEqual(result,[{'counter':count,'node':i} for i in range(4)])
   with self.assertRaisesRegex(RuntimeError,'expected remote rejection'):sessions[0].call({'fail':True})
   self.assertEqual(sessions[0].call({'node':0})['counter'],4)
  finally:
   for session in sessions:session.close()
if __name__=='__main__':unittest.main(verbosity=2)
