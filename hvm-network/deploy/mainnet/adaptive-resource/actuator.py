#!/usr/bin/env python3
"""HashBurst Adaptive Resource Actuator v1 - simulation backend only."""
import argparse,datetime,hashlib,json,pathlib,sys

MAX_TTL=300
MIN_BUDGET_DELTA=2

def canonical(o): return json.dumps(o,sort_keys=True,separators=(",",":"))
def sha(o): return hashlib.sha256(canonical(o).encode()).hexdigest()
def parse_time(s):
    if not isinstance(s,str) or not s.endswith("Z"): raise ValueError("timestamp must be UTC Z")
    return datetime.datetime.fromisoformat(s[:-1]+"+00:00")
def fail(m): raise SystemExit("ACTUATOR_REJECTED: "+m)
def verify_decision(d):
    got=d.get("decision_sha256")
    body={k:v for k,v in d.items() if k!="decision_sha256"}
    if not got or sha(body)!=got: fail("decision digest mismatch")
    if d.get("schema")!="hashburst-adaptive-resource-decision-v1" or d.get("mode")!="dry-run": fail("decision schema/mode")
    if d.get("proposal",{}).get("actuation")!="none": fail("source decision already requests actuation")
    for u in ("hashburst-hvm-testnet.service","hashburst-tep.service"):
        if d.get("protected",{}).get("services",{}).get(u)!="active": fail("protected service unhealthy: "+u)
def main():
    ap=argparse.ArgumentParser();ap.add_argument("--now",required=True);ap.add_argument("--state",required=True);a=ap.parse_args()
    env=json.load(sys.stdin); now=parse_time(a.now)
    if env.get("schema")!="hashburst-actuation-envelope-v1": fail("envelope schema")
    issued=parse_time(env.get("issued_at")); expires=parse_time(env.get("expires_at"))
    if expires<=issued or (expires-issued).total_seconds()>MAX_TTL: fail("invalid ttl")
    if now<issued or now>expires: fail("expired/not-yet-valid")
    d=env.get("decision",{});verify_decision(d)
    if env.get("decision_sha256")!=d.get("decision_sha256"): fail("envelope decision binding")
    nonce=env.get("nonce")
    if not isinstance(nonce,str) or len(nonce)<16: fail("nonce")
    p=pathlib.Path(a.state); state=json.loads(p.read_text()) if p.exists() else {"consumed":[],"current_budget":None}
    if nonce in state.get("consumed",[]): fail("replay")
    budget=int(d["proposal"]["mining_cpu_budget"]); current=state.get("current_budget")
    action="simulate-apply";reason="budget_change"
    if current is not None and abs(budget-int(current))<MIN_BUDGET_DELTA:
        action="hold";reason="hysteresis"
    sim=env.get("simulation",{})
    before=sim.get("before_health",{});after=sim.get("after_health",{})
    for u in ("hashburst-hvm-testnet.service","hashburst-tep.service"):
        if before.get(u)!="active": fail("pre-health failed: "+u)
    result="held" if action=="hold" else "committed"
    rollback=False
    if action=="simulate-apply" and any(after.get(u)!="active" for u in ("hashburst-hvm-testnet.service","hashburst-tep.service")):
        result="rolled_back";rollback=True
    state.setdefault("consumed",[]).append(nonce)
    if result=="committed": state["current_budget"]=budget
    p.write_text(canonical(state)+"\n")
    out={"schema":"hashburst-actuator-result-v1","backend":"simulation","node":d.get("node"),
         "decision_sha256":d["decision_sha256"],"nonce":nonce,"action":action,"reason":reason,
         "requested_budget":budget,"result":result,"rollback":rollback,
         "systemd_mutation":False,"miner_restart":False}
    out["result_sha256"]=sha(out)
    print(canonical(out))
if __name__=="__main__": main()
