import json, pathlib, subprocess, sys, tempfile, unittest
class SamplesTest(unittest.TestCase):
 def test_cpu_and_restart(self):
  def sample(ns,pid,start,cpu):
   fields=['S']+['0']*21
   fields[11]=str(cpu);fields[12]='0';fields[19]=str(start)
   return {'monotonic_ns':ns,'clock_ticks':100,'service':{'MainPID':str(pid)},'stat':f'{pid} (name with ) bracket) '+' '.join(fields),'status':'VmRSS: 100 kB\n','io':'read_bytes: 20\n','cgroup_cpu.stat':'usage_usec 10\n'}
  values=[sample(0,7,100,20),sample(1000000000,7,100,70),sample(2000000000,8,101,3)]
  with tempfile.TemporaryDirectory() as d:
   path=pathlib.Path(d)/'samples';path.write_text('\n'.join(map(json.dumps,values)))
   result=subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name('analyze-samples.py')),str(path)],check=True,capture_output=True,text=True)
   out=json.loads(result.stdout)
  self.assertEqual(out['intervals'][0]['process_cpu_seconds'],.5)
  self.assertFalse(out['intervals'][1]['same_process'])
  self.assertNotIn('process_cpu_seconds',out['intervals'][1])
  self.assertIsNone(out['energy_joules'])
if __name__=='__main__':unittest.main()
