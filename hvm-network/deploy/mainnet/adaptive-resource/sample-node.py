#!/usr/bin/env python3
"""Read-only service/miner sampler for canary/control comparison."""
import json,os,subprocess,time
def out(a):
 return subprocess.run(a,text=True,capture_output=True,timeout=5).stdout.strip()
def active(u): return out(["systemctl","is-active",u])
def mainpid(u):
 try:return int(out(["systemctl","show",u,"-p","MainPID","--value"]) or 0)
 except:return 0
def pcpu(pid):
 if pid<=0:return 0.0
 try:return float(out(["ps","-p",str(pid),"-o","pcpu="]) or 0)
 except:return 0.0
def commsum(name):
 s=0.0
 for x in out(["ps","-C",name,"-o","pcpu="]).splitlines():
  try:s+=float(x)
  except:pass
 return s
def shares():
 # Do not fabricate. Only expose if a known local cpuminer log provides parseable counters.
 candidates=["/var/log/cpuminer.log","/var/log/hashburst/cpuminer.log"]
 for p in candidates:
  if not os.path.isfile(p): continue
  try:
   lines=open(p,errors="replace").read().splitlines()[-2000:]
   acc=sum(1 for x in lines if "accepted" in x.lower())
   rej=sum(1 for x in lines if "rejected" in x.lower())
   if acc or rej:return {"source":p,"accepted_events_window":acc,"rejected_events_window":rej,"quality":"log-event-proxy"}
  except:pass
 return {"source":None,"accepted_events_window":None,"rejected_events_window":None,"quality":"unavailable"}
hvm="hashburst-hvm-testnet.service";tep="hashburst-tep.service"
print(json.dumps({"schema":"hashburst-canary-sample-v1","ts":time.time(),"node":out(["hostname"]),
 "cpu_count":os.cpu_count(),"load1":float(open("/proc/loadavg").read().split()[0]),
 "miner_cpu_percent":commsum("cpuminer"),
 "hvm":{"unit":hvm,"state":active(hvm),"main_pid":mainpid(hvm),"cpu_percent":pcpu(mainpid(hvm))},
 "tep":{"unit":tep,"state":active(tep),"main_pid":mainpid(tep),"cpu_percent":pcpu(mainpid(tep))},
 "shares":shares()},sort_keys=True,separators=(",",":")))
