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

class HostCountersTest(unittest.TestCase):
 def test_steal_guest_and_resets(self):
  import importlib.util
  spec=importlib.util.spec_from_file_location('analyzer',pathlib.Path(__file__).with_name('analyze-samples.py'))
  m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  x={'monotonic_ns':0,'host_stat':'cpu 10 0 0 0 0 0 0 20 999 999',
     'cgroup_cpu.pressure':'some avg10=0 total=100\nfull avg10=0 total=50'}
  y={'monotonic_ns':1000000000,'host_stat':'cpu 25 0 0 0 0 0 0 105 9999 9999',
     'cgroup_cpu.pressure':'some avg10=0 total=10100\nfull avg10=0 total=50'}
  row=m.interval(x,y)
  self.assertEqual(row['host_steal_percent'],85)
  self.assertEqual(row['cpu_pressure_percent']['some'],1)
  y['host_stat']='cpu 0 0 0 0 0 0 0 0'
  self.assertNotIn('host_steal_percent',m.interval(x,y))
  with self.assertRaises(ValueError):m.interval(x,x)

if __name__=='__main__':unittest.main()
