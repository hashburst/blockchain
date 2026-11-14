#!/usr/bin/env python3
import hashlib,json,os,subprocess,sys
from pathlib import Path
out=Path(sys.argv[1]).resolve()
root=Path(__file__).resolve().parents[2]
artifacts=["hashburst-mainnet","hashburst-testnet","hvm-mainnet-evm-gateway"]
sha=lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
source=(out/"SOURCE_COMMIT").read_text().strip()
mods=json.loads(subprocess.check_output(["go","list","-m","-json","all"],cwd=root,text=True).replace("}\n{","},\n{").join(["[","]"])) if False else None
raw=subprocess.check_output(["go","list","-m","-json","all"],cwd=root,text=True)
dec=json.JSONDecoder(); pos=0; components=[]
while pos < len(raw):
    while pos<len(raw) and raw[pos].isspace(): pos+=1
    if pos>=len(raw): break
    obj,n=dec.raw_decode(raw,pos); pos=n
    components.append({"type":"library","name":obj["Path"],"version":obj.get("Version") or obj.get("Main",{}).get("Version") or source})
sbom={"bomFormat":"CycloneDX","specVersion":"1.5","version":1,
      "metadata":{"component":{"type":"application","name":"HashBurst-HVM-G5","version":source}},
      "components":components}
(out/"SBOM.cdx.json").write_text(json.dumps(sbom,sort_keys=True,separators=(",",":"))+"\n")
manifest={"schema":"hashburst-g5-manifest-v1","source_commit":source,"target":"linux-amd64",
          "artifacts":[{"name":x,"sha256":sha(out/x)} for x in artifacts],
          "sbom_sha256":sha(out/"SBOM.cdx.json")}
(out/"MANIFEST.json").write_text(json.dumps(manifest,sort_keys=True,separators=(",",":"))+"\n")
gov=subprocess.check_output(["go","version"],text=True).strip()
prov={"schema":"hashburst-g5-original-build-provenance-v1","original_build_provenance":True,
      "repository":"hashburst/blockchain","source_commit":source,"builder":"github-actions",
      "workflow":"hvm-g5-supply-chain.yml","go_version":gov,"goos":"linux","goarch":"amd64",
      "cgo_enabled":"0","build_flags":["-buildvcs=false","-trimpath","-ldflags=-buildid="],
      "artifacts":[{"name":x,"sha256":sha(out/x)} for x in artifacts],
      "manifest_sha256":sha(out/"MANIFEST.json"),"sbom_sha256":sha(out/"SBOM.cdx.json")}
(out/"PROVENANCE.json").write_text(json.dumps(prov,sort_keys=True,separators=(",",":"))+"\n")
with (out/"SHA256SUMS").open("w") as f:
    for x in artifacts+["SOURCE_COMMIT","SBOM.cdx.json","MANIFEST.json","PROVENANCE.json"]:
        f.write(f"{sha(out/x)}  {x}\n")
