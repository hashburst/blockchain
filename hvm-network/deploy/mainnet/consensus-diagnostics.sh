#!/usr/bin/env bash
set -euo pipefail

KNOWN=/etc/hashburst-control/fleet_known_hosts
SOCK=/run/hashburst-control/ssh-agent.sock
HOSTS=(77.90.188.153 77.90.188.154 77.90.188.155 77.90.188.157)
STATE=/var/lib/hashburst-hvm-mainnet
OPT=/opt/hashburst-hvm-mainnet
SERVICE=hashburst-hvm-mainnet.service
EXPECTED_BINARY_SHA=5a0cc087d757b7659698f92459490ae15bdf4471763360d6d2a0098545268e94
OUT=mainnet-r7b-consensus-diagnostics.json
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

test -S "$SOCK"
test -r "$KNOWN"

for ip in "${HOSTS[@]}"; do
  SSH_AUTH_SOCK="$SOCK" ssh -T     -o UserKnownHostsFile="$KNOWN"     -o StrictHostKeyChecking=yes     -o BatchMode=yes     root@"$ip"     python3 - "$STATE" "$OPT" "$SERVICE" "$EXPECTED_BINARY_SHA" <<'PY' > "$TMP/$ip.json"
import collections, hashlib, json, os, pathlib, subprocess, sys

state=pathlib.Path(sys.argv[1]); opt=pathlib.Path(sys.argv[2])
service=sys.argv[3]; expected_binary=sys.argv[4]

def cmd(args):
    p=subprocess.run(args,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    return {"rc":p.returncode,"out":p.stdout.strip()}

def sha256(path):
    h=hashlib.sha256()
    with open(path,"rb") as f:
        for chunk in iter(lambda:f.read(1024*1024),b""):
            h.update(chunk)
    return h.hexdigest()

def stat_regular(path):
    st=os.lstat(path)
    return {
        "regular": pathlib.Path(path).is_file() and not pathlib.Path(path).is_symlink(),
        "mode": oct(st.st_mode & 0o777),
        "size": st.st_size,
        "sha256": sha256(path),
    }

active=cmd(["systemctl","is-active",service])
enabled=cmd(["systemctl","is-enabled",service])
if active["out"] == "active":
    raise SystemExit("refusing diagnostics while runtime is active")

binary=opt/"hashburst-mainnet"
binary_sha=sha256(binary)
if binary_sha != expected_binary:
    raise SystemExit("installed binary digest mismatch")

bft=state/"consensus-bft-signatures.jsonl"
votes=state/"consensus-votes.jsonl"
recovery=state/"consensus-recovery.json"
pin=state/"runtime.pin"

records=[]
with open(bft,encoding="utf-8") as f:
    for n,line in enumerate(f,1):
        line=line.strip()
        if not line: continue
        r=json.loads(line)
        r["_line"]=n
        records.append(r)

h9=[r for r in records if int(r.get("height",0))==9]
post64=[r for r in h9 if int(r.get("round",0))>64]
steps=collections.Counter(str(r.get("step","")) for r in h9)
steps_post64=collections.Counter(str(r.get("step","")) for r in post64)
rounds=collections.defaultdict(lambda:collections.Counter())
hashes=collections.Counter()
for r in h9:
    rd=int(r.get("round",0)); step=str(r.get("step",""))
    rounds[rd][step]+=1
    bh=str(r.get("block_hash",""))
    if bh: hashes[bh]+=1

rec_raw=json.load(open(recovery,encoding="utf-8"))
payload=rec_raw.get("Payload",rec_raw.get("payload"))
if isinstance(payload,str):
    rec=json.loads(payload)
elif isinstance(payload,dict):
    rec=payload
else:
    rec={}

journal=cmd([
    "journalctl","-u",service,"--since","2026-11-09 10:40:00 UTC",
    "--until","2026-11-09 11:05:00 UTC","--no-pager","-o","short-iso"
])["out"].splitlines()
terms=("CONSENS","ROUND","PREVOTE","PRECOMMIT","PROPOS","FINAL","PEER","RECOVER","BFT","HEIGHT")
journal_focus=[line for line in journal if any(t in line.upper() for t in terms)]

out={
 "schema":1,
 "mode":"read-only",
 "host":cmd(["hostname"])["out"],
 "service":{"active":active["out"],"enabled":enabled["out"]},
 "installed_binary_sha256":binary_sha,
 "files":{
   "bft_journal":stat_regular(bft),
   "vote_journal":stat_regular(votes),
   "recovery":stat_regular(recovery),
   "runtime_pin":stat_regular(pin),
 },
 "recovery":{
   "Version":rec.get("Version"),"ChainID":rec.get("ChainID"),
   "Height":rec.get("Height"),"ReservedRound":rec.get("ReservedRound"),
   "ValidatorID":rec.get("ValidatorID"),"ParentHash":rec.get("ParentHash"),
   "ConfigHash":rec.get("ConfigHash"),"LockedRound":rec.get("LockedRound"),
   "ValidRound":rec.get("ValidRound"),"LockedHash":rec.get("LockedHash"),
   "ValidHash":rec.get("ValidHash"),
   "has_locked_block":rec.get("LockedBlock") is not None,
   "has_valid_block":rec.get("ValidBlock") is not None,
   "has_locked_qc":rec.get("LockedQC") is not None,
   "has_valid_qc":rec.get("ValidQC") is not None,
 },
 "height9":{
   "records":len(h9),
   "post_round64_records":len(post64),
   "min_round":min((int(r.get("round",0)) for r in h9),default=None),
   "max_round":max((int(r.get("round",0)) for r in h9),default=None),
   "steps":dict(sorted(steps.items())),
   "post_round64_steps":dict(sorted(steps_post64.items())),
   "block_hashes":dict(sorted(hashes.items())),
   "rounds":{str(k):dict(sorted(v.items())) for k,v in sorted(rounds.items())},
 },
 "journal_focus":journal_focus[-400:],
}
print(json.dumps(out,separators=(",",":"),sort_keys=True))
PY
  echo "HVM_R7B_CONSENSUS_DIAGNOSTIC_HOST_OK ip=$ip"
done

python3 - "$TMP" "$OUT" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1])
nodes=[]
for p in sorted(root.glob("*.json")):
    nodes.append(json.load(open(p,encoding="utf-8")))
assert len(nodes)==4
json.dump({"schema":1,"mode":"read-only","nodes":nodes},open(sys.argv[2],"w",encoding="utf-8"),indent=2,sort_keys=True)
open(sys.argv[2],"a",encoding="utf-8").write("\n")
PY

cat "$OUT"
echo HVM_R7B_CONSENSUS_DIAGNOSTICS_READONLY_4OF4_OK
