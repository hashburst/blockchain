#!/usr/bin/env python3
"""Read-only TESTNET preflight on a target Linux VPS. Does not approve a release."""
import argparse, hashlib, json, subprocess, urllib.request
from pathlib import Path

def sha(path):
    h=hashlib.sha256()
    with open(path,'rb') as f:
        for b in iter(lambda:f.read(1048576),b''):h.update(b)
    return h.hexdigest()

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--node',required=True,choices=['hvm-testnet-v'+str(i) for i in range(1,5)]+['hvm-testnet-ingress'])
    a=p.parse_args();stem='hashburst-hvm-testnet'+('-ingress' if a.node.endswith('ingress') else '')
    c=json.loads((Path('/etc')/stem/'node.json').read_text())
    if c.get('node_id')!=a.node or c.get('network')!='testnet' or c.get('protocol',{}).get('chain_id')!=4735490:raise RuntimeError('identity/network mismatch')
    raw=subprocess.run(['systemctl','show',stem+'.service','--property=MainPID,ActiveState,ExecStart,NRestarts'],check=True,capture_output=True,text=True,timeout=15).stdout
    props=dict(x.split('=',1) for x in raw.splitlines() if '=' in x)
    pid=int(props.get('MainPID','0'))
    if pid<=0:raise RuntimeError('no running predecessor; inspect journal before installing')
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open('http://127.0.0.1:18009/health',timeout=10) as f:health=json.load(f)
    if health.get('node_id')!=a.node or health.get('chain_id')!=4735490 or health.get('peer_id')!=c.get('peer_id'):raise RuntimeError('health identity mismatch')
    print(json.dumps({'node':a.node,'service':props,'running_sha256':sha('/proc/'+str(pid)+'/exe'),'health':health,'no_service_changed':True,'predecessor_approved':False},indent=2))
if __name__=='__main__':
    try:main()
    except Exception as e:raise SystemExit('STOP: '+str(e)+'; no service changed')
