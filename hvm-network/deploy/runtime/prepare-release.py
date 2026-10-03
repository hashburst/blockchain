#!/usr/bin/env python3
"""Build a TESTNET installer manifest from explicitly reviewed predecessor hashes."""
import argparse, hashlib, json, re
from pathlib import Path

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--predecessor',action='append',required=True,help='reviewed SHA256 of an existing runtime; repeat for multiple versions')
    p.add_argument('--directory',type=Path,default=Path(__file__).resolve().parent)
    a=p.parse_args()
    for h in a.predecessor:
        if not re.fullmatch('[0-9a-f]{64}',h):p.error('predecessor must be 64 lowercase hexadecimal characters')
    root=a.directory
    commit=(root/'SOURCE_COMMIT').read_text().strip()
    if not re.fullmatch('[0-9a-f]{40}',commit):p.error('invalid SOURCE_COMMIT')
    h=hashlib.sha256()
    with (root/'hashburst-testnet').open('rb') as f:
        for chunk in iter(lambda:f.read(1048576),b''):h.update(chunk)
    release={'chain_id':4735490,'source_commit':commit,'sha256':h.hexdigest(),'predecessors':sorted(set(a.predecessor))}
    output=root/'release.json'
    with output.open('x') as f:json.dump(release,f,indent=2);f.write('\n')
    print('MANIFEST_CREATED_NO_SERVICE_CHANGED='+str(output))
if __name__=='__main__':main()
