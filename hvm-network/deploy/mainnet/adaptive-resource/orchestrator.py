#!/usr/bin/env python3
"""HashBurst Adaptive Resource Orchestrator v1: deterministic dry-run policy.

Reads a bounded JSON telemetry document from stdin and emits a decision only.
It never invokes systemctl, writes cgroups, signals miners, or changes consensus.
"""
import json,sys,hashlib,datetime

POLICY={
 "schema":"hashburst-adaptive-resource-policy-v1",
 "version":"1.0.0",
 "protected_services":["hashburst-hvm-testnet.service","hashburst-tep.service"],
 "minimum_reserved_cpus":8,
 "normal_mining_fraction":2/3,
 "minimum_mining_cpus":4,
 "load_warn_per_cpu":0.85,
 "load_critical_per_cpu":1.10,
 "hvm_cpu_warn_fraction":0.25,
 "tep_cpu_warn_fraction":0.20
}
def die(m): raise SystemExit("ORCHESTRATOR_INVALID: "+m)
d=json.load(sys.stdin)
cpu=int(d.get("cpu_count",0))
if cpu<1: die("cpu_count")
services=d.get("services",{})
for u in POLICY["protected_services"]:
 if services.get(u)!="active": die("protected service not active: "+u)
load1=float(d.get("load1",0))
miner=float(d.get("miner_cpu_percent",0))
hvm=float(d.get("hvm_cpu_percent",0))
tep=float(d.get("tep_cpu_percent",0))
reserve=max(POLICY["minimum_reserved_cpus"],cpu-round(cpu*POLICY["normal_mining_fraction"]))
normal=max(POLICY["minimum_mining_cpus"],cpu-reserve)
reasons=[]
budget=normal
ratio=load1/cpu
if ratio>=POLICY["load_critical_per_cpu"]:
 budget=max(POLICY["minimum_mining_cpus"],normal-8);reasons.append("critical_host_load")
elif ratio>=POLICY["load_warn_per_cpu"]:
 budget=max(POLICY["minimum_mining_cpus"],normal-4);reasons.append("high_host_load")
if hvm/cpu/100>=POLICY["hvm_cpu_warn_fraction"]:
 budget=max(POLICY["minimum_mining_cpus"],budget-2);reasons.append("hvm_cpu_pressure")
if tep/cpu/100>=POLICY["tep_cpu_warn_fraction"]:
 budget=max(POLICY["minimum_mining_cpus"],budget-2);reasons.append("tep_cpu_pressure")
if not reasons: reasons=["normal_protected_capacity"]
decision={
 "schema":"hashburst-adaptive-resource-decision-v1","mode":"dry-run",
 "node":d.get("node"),"cpu_count":cpu,"observed":{"load1":load1,"load_per_cpu":ratio,"miner_cpu_percent":miner,"hvm_cpu_percent":hvm,"tep_cpu_percent":tep},
 "protected":{"reserved_cpus":cpu-budget,"services":services},
 "proposal":{"mining_cpu_budget":budget,"cpu_quota_percent":budget*100,"nice":10,"actuation":"none"},
 "reason_codes":reasons,
 "energy_accounting":{"measurement_quality":d.get("energy_measurement_quality","proxy"),"joules":None,"co2e_g":None,"claim":"not_measured"},
 "policy_version":POLICY["version"]
}
canonical=json.dumps(decision,sort_keys=True,separators=(",",":"))
decision["decision_sha256"]=hashlib.sha256(canonical.encode()).hexdigest()
print(json.dumps(decision,sort_keys=True,separators=(",",":")))
