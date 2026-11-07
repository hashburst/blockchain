#!/usr/bin/env python3
import datetime,hashlib,json,pathlib,subprocess,tempfile
ROOT=pathlib.Path(__file__).parent; ACT=ROOT/"actuator.py"
def digest(d):
 b={k:v for k,v in d.items() if k!="decision_sha256"}
 return hashlib.sha256(json.dumps(b,sort_keys=True,separators=(",",":")).encode()).hexdigest()
def decision(budget=12):
 d={"schema":"hashburst-adaptive-resource-decision-v1","mode":"dry-run","node":"test",
 "protected":{"reserved_cpus":24-budget,"services":{"hashburst-hvm-testnet.service":"active","hashburst-tep.service":"active"}},
 "proposal":{"mining_cpu_budget":budget,"cpu_quota_percent":budget*100,"nice":10,"actuation":"none"},
 "observed":{},"reason_codes":["high_host_load"],"policy_version":"1.0.0","energy_accounting":{},"cpu_count":24}
 d["decision_sha256"]=digest(d);return d
NOW="2026-11-07T15:00:30Z"
def env(d,nonce,after=("active","active"),issued="2026-11-07T15:00:00Z",expires="2026-11-07T15:03:00Z"):
 return {"schema":"hashburst-actuation-envelope-v1","issued_at":issued,"expires_at":expires,"nonce":nonce,
 "decision_sha256":d["decision_sha256"],"decision":d,
 "simulation":{"before_health":{"hashburst-hvm-testnet.service":"active","hashburst-tep.service":"active"},
 "after_health":{"hashburst-hvm-testnet.service":after[0],"hashburst-tep.service":after[1]}}}
def run(e,state,ok=True,now=NOW):
 r=subprocess.run(["python3",str(ACT),"--now",now,"--state",str(state)],input=json.dumps(e),text=True,capture_output=True)
 assert (r.returncode==0)==ok,(r.stdout,r.stderr)
 return json.loads(r.stdout) if ok else r
with tempfile.TemporaryDirectory() as td:
 s=pathlib.Path(td)/"state.json"
 d=decision(12); e=env(d,"0123456789abcdef-A")
 r=run(e,s);assert r["result"]=="committed" and not r["systemd_mutation"]
 run(e,s,False) # replay
 d2=decision(13);r=run(env(d2,"0123456789abcdef-B"),s);assert r["result"]=="held" # hysteresis
 d3=decision(8);r=run(env(d3,"0123456789abcdef-C",("active","failed")),s);assert r["result"]=="rolled_back" and r["rollback"]
 bad=env(decision(8),"0123456789abcdef-D");bad["decision"]["proposal"]["mining_cpu_budget"]=20
 run(bad,s,False) # tamper
 expired=env(decision(8),"0123456789abcdef-E",issued="2026-11-07T14:00:00Z",expires="2026-11-07T14:03:00Z")
 run(expired,s,False)
print("HASHBURST_ADAPTIVE_RESOURCE_ACTUATOR_SIMULATION_TESTS_OK")
print("HASHBURST_ADAPTIVE_RESOURCE_ANTI_REPLAY_TESTS_OK")
print("HASHBURST_ADAPTIVE_RESOURCE_ROLLBACK_SIMULATION_OK")
print("HASHBURST_ADAPTIVE_RESOURCE_HYSTERESIS_TESTS_OK")
