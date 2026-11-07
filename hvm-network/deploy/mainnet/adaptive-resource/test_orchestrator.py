#!/usr/bin/env python3
import json,subprocess,pathlib
p=pathlib.Path(__file__).with_name("orchestrator.py")
def run(load,miner=2300,hvm=25,tep=12):
 d={"node":"test","cpu_count":24,"load1":load,"miner_cpu_percent":miner,"hvm_cpu_percent":hvm,"tep_cpu_percent":tep,
 "services":{"hashburst-hvm-testnet.service":"active","hashburst-tep.service":"active"},"energy_measurement_quality":"proxy"}
 r=subprocess.run(["python3",str(p)],input=json.dumps(d),text=True,capture_output=True)
 assert r.returncode==0,r.stderr
 return json.loads(r.stdout)
assert run(10)["proposal"]["mining_cpu_budget"]==16
assert run(24)["proposal"]["mining_cpu_budget"]==12
assert run(28)["proposal"]["mining_cpu_budget"]==8
bad={"cpu_count":24,"load1":24,"services":{"hashburst-hvm-testnet.service":"failed","hashburst-tep.service":"active"}}
r=subprocess.run(["python3",str(p)],input=json.dumps(bad),text=True,capture_output=True)
assert r.returncode!=0
print("HASHBURST_ADAPTIVE_RESOURCE_POLICY_TESTS_OK")
