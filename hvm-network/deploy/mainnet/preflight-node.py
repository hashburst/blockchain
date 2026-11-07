#!/usr/bin/env python3
"""Read-only mainnet readiness inventory. Never reads private key contents."""
import json,os,platform,socket,subprocess,sys
from pathlib import Path

def cmd(args):
    p=subprocess.run(args,text=True,capture_output=True,timeout=8)
    return p.returncode,p.stdout.strip()

def check(expected_ip,role):
    r={'schema':1,'network':'hvm-mainnet-candidate','chain_id':4735489,'expected_ip':expected_ip,'role':role,'hostname':socket.gethostname(),'architecture':platform.machine(),'errors':[]}
    if platform.system()!='Linux' or platform.machine() not in ('x86_64','amd64'):r['errors'].append('Linux amd64 required')
    if os.geteuid()!=0:r['errors'].append('root required for inventory')
    rc,out=cmd(['ip','-j','-4','address','show'])
    try: local=[a['local'] for x in json.loads(out) for a in x.get('addr_info',[]) if a.get('family')=='inet'] if rc==0 else []
    except Exception: local=[]
    r['expected_ip_present']=expected_ip in local
    if not r['expected_ip_present']:r['errors'].append('expected IPv4 not assigned locally')
    r['existing_mainnet_paths']=[p for p in ('/etc/hashburst-hvm-mainnet','/var/lib/hashburst-hvm-mainnet','/root/hvm-mainnet-identity') if Path(p).exists() or Path(p).is_symlink()]
    r['existing_testnet_paths']=[p for p in ('/etc/hashburst-hvm-testnet','/var/lib/hashburst-hvm-testnet','/root/hvm-testnet-identity') if Path(p).exists() or Path(p).is_symlink()]
    r['units']={}
    for u in ('hashburst-hvm-mainnet.service','hashburst-hvm-testnet.service','hashburst-tep.service'):
        rc,out=cmd(['systemctl','show',u,'-p','ActiveState','--value']);r['units'][u]=out or 'unknown'
    if r['units']['hashburst-hvm-mainnet.service'] in ('active','activating','reloading'):r['errors'].append('unexpected mainnet service already active')
    r['ports']={}
    for port in (18010,31308):
        with socket.socket() as s:
            try:s.bind(('0.0.0.0',port));r['ports'][str(port)]='free'
            except OSError:r['ports'][str(port)]='in-use'
    if role=='validator' and any(v!='free' for v in r['ports'].values()):r['errors'].append('candidate mainnet port unavailable')
    r['ok']=not r['errors'];return r
if __name__=='__main__':
    try:
        if len(sys.argv)!=3 or sys.argv[2] not in ('validator','observer'):raise ValueError()
        report=check(sys.argv[1],sys.argv[2])
    except Exception as e:report={'ok':False,'errors':['preflight failed: '+type(e).__name__]}
    print(json.dumps(report,indent=2));sys.exit(0 if report['ok'] else 1)
