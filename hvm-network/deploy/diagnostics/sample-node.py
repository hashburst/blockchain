#!/usr/bin/env python3
"""Read-only Linux process/cgroup sampling. No restart, signals, or key/config reads."""
import argparse, json, pathlib, subprocess, time

p=argparse.ArgumentParser()
p.add_argument('--unit',default='hashburst-hvm-testnet.service')
p.add_argument('--seconds',type=int,default=60)
a=p.parse_args()
if not 1<=a.seconds<=300: p.error('seconds must be 1..300')
if not a.unit.endswith('.service') or '/' in a.unit or a.unit.startswith('-'): p.error('invalid service unit')
def read(path):
    try: return pathlib.Path(path).read_text()
    except OSError as e: return {'unavailable':str(e)}
deadline=time.monotonic()+a.seconds
while True:
    r=subprocess.run(['systemctl','show',a.unit,'-p','MainPID','-p','ControlGroup','-p','NRestarts','-p','ActiveState'],text=True,capture_output=True,timeout=5,check=True)
    props=dict(line.split('=',1) for line in r.stdout.splitlines() if '=' in line)
    pid=int(props.get('MainPID','0'));out={'time_ns':time.time_ns(),'monotonic_ns':time.monotonic_ns(),'service':props}
    if pid>0:
        for n in ('stat','status','io','schedstat','smaps_rollup'):
            out[n]=read(f'/proc/{pid}/{n}')
    out['host_stat']=read('/proc/stat')
    cg=props.get('ControlGroup','')
    if cg.startswith('/') and '..' not in cg.split('/'):
        for n in ('cpu.stat','cpu.max','memory.current','memory.events','io.stat','cpu.pressure','io.pressure','memory.pressure'):
            out['cgroup_'+n]=read('/sys/fs/cgroup'+cg+'/'+n)
    print(json.dumps(out),flush=True)
    left=deadline-time.monotonic()
    if left<=0: break
    time.sleep(min(5,left))
