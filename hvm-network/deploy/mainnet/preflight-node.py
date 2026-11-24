#!/usr/bin/env python3
"""Read-only HVM mainnet production federation preflight. Never mutates services."""
import json,os,platform,socket,subprocess,sys

CHAIN_ID=4735489
CONFIG_DIGEST='86a9c1e1ed65c2e3afcaf469a3155fad94add9e88a2b90e5bc80e0c2e33b3fb3'
SERVICE='hashburst-hvm-mainnet.service'
HEALTH='http://127.0.0.1:18019/health'

def cmd(args):
    p=subprocess.run(args,text=True,capture_output=True,timeout=8)
    return p.returncode,p.stdout.strip()

def check(expected_ip,role):
    r={'schema':2,'network':'hvm-mainnet-production','chain_id':CHAIN_ID,'expected_ip':expected_ip,'role':role,'hostname':socket.gethostname(),'architecture':platform.machine(),'errors':[]}
    if platform.system()!='Linux' or platform.machine() not in ('x86_64','amd64'):r['errors'].append('Linux amd64 required')
    if os.geteuid()!=0:r['errors'].append('root required for inventory')
    rc,out=cmd(['ip','-j','-4','address','show'])
    try: local=[a['local'] for x in json.loads(out) for a in x.get('addr_info',[]) if a.get('family')=='inet'] if rc==0 else []
    except Exception: local=[]
    r['expected_ip_present']=expected_ip in local
    if not r['expected_ip_present']:r['errors'].append('expected IPv4 not assigned locally')
    rc,active=cmd(['systemctl','is-active',SERVICE]);r['service_active']=active
    if rc!=0 or active!='active':r['errors'].append('mainnet service not active')
    rc,enabled=cmd(['systemctl','is-enabled',SERVICE]);r['service_enabled']=enabled
    if rc!=0 or enabled!='enabled':r['errors'].append('mainnet service not enabled')
    rc,body=cmd(['curl','-fsS','--max-time','5',HEALTH])
    try:
        h=json.loads(body) if rc==0 else {}
    except Exception:
        h={}
    r['health']={k:h.get(k) for k in ('ok','network','chain_id','config_digest','role','height','finalized_height','peer_count','reactor_running','reactor_status_fresh')}
    if rc!=0 or not h:r['errors'].append('mainnet health unavailable')
    else:
        if h.get('ok') is not True:r['errors'].append('health ok != true')
        if h.get('network')!='mainnet':r['errors'].append('network mismatch')
        if h.get('chain_id')!=CHAIN_ID:r['errors'].append('chain_id mismatch')
        if h.get('config_digest')!=CONFIG_DIGEST:r['errors'].append('config_digest mismatch')
        if h.get('role')!='validator':r['errors'].append('validator role required')
        if not isinstance(h.get('height'),int) or not isinstance(h.get('finalized_height'),int) or h.get('height',-1)<h.get('finalized_height',0):r['errors'].append('invalid height/finality')
        if not isinstance(h.get('peer_count'),int) or h.get('peer_count',-1)<0:r['errors'].append('invalid peer_count')
        if h.get('reactor_running') is not True:r['errors'].append('reactor not running')
        if h.get('reactor_status_fresh') is not True:r['errors'].append('reactor status stale')
    r['ok']=not r['errors']
    return r

if __name__=='__main__':
    try:
        if len(sys.argv)!=3 or sys.argv[2]!='validator':raise ValueError()
        report=check(sys.argv[1],sys.argv[2])
    except Exception as e:
        report={'ok':False,'errors':['preflight failed: '+type(e).__name__]}
    print(json.dumps(report,indent=2))
    sys.exit(0 if report['ok'] else 1)
