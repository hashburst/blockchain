#!/usr/bin/env python3
"""Wrap an orchestrator decision in a short-lived actuation authorization envelope."""
import argparse,datetime,json,secrets,sys
ap=argparse.ArgumentParser();ap.add_argument("--issued-at",required=True);ap.add_argument("--ttl",type=int,default=180)
ap.add_argument("--after-hvm",default="active");ap.add_argument("--after-tep",default="active");a=ap.parse_args()
if not 1<=a.ttl<=300: raise SystemExit("ttl must be 1..300")
issued=datetime.datetime.fromisoformat(a.issued_at.replace("Z","+00:00"))
d=json.load(sys.stdin)
out={"schema":"hashburst-actuation-envelope-v1","issued_at":a.issued_at,
 "expires_at":(issued+datetime.timedelta(seconds=a.ttl)).isoformat().replace("+00:00","Z"),
 "nonce":secrets.token_hex(16),"decision_sha256":d.get("decision_sha256"),"decision":d,
 "simulation":{"before_health":{"hashburst-hvm-testnet.service":"active","hashburst-tep.service":"active"},
 "after_health":{"hashburst-hvm-testnet.service":a.after_hvm,"hashburst-tep.service":a.after_tep}}}
print(json.dumps(out,sort_keys=True,separators=(",",":")))
