#!/usr/bin/env python3
import hashlib,json,os,pwd,socket,subprocess,sys,tempfile,urllib.request
from pathlib import Path
UNIT='hashburst-hvm-testnet.service'
BINARY='/opt/hashburst-hvm-testnet/releases/06d7c3bd2cff59b2e95b25ed09fe8509dd6de01a/hashburst-testnet'
DIGEST='502c051c2cf6040b988fe2bc94ae984ab7f709e059918237110af678266ea215'
DATA=Path('/var/lib/hashburst-hvm-testnet')
ETC=Path('/etc/hashburst-hvm-testnet')
def require(ok,msg):
    if not ok: raise RuntimeError(msg)
def run(args):return subprocess.run(args,check=True,stdout=sys.stderr,timeout=300)
def get(path='/health',body=None):
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    req=urllib.request.Request('http://127.0.0.1:18009'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json'})
    with opener.open(req,timeout=5) as r:return json.load(r)
def health(expected,role):
    d=get()
    for key,value in {'ok':True,'network':'testnet','chain_id':4735490,'node_id':expected['node_id'],'peer_id':expected['peer_id'],'config_digest':DIGEST,'role':role}.items():require(d.get(key)==value,'health mismatch: '+key)
    require(type(d.get('finalized_height')) is int and d['finalized_height']>=7,'invalid finalized height')
    require(d.get('reactor_running') is True,'reactor not running')
    # TryStatus uses TryLock: false means no snapshot, not a stopped reactor.
    require(d.get('reactor',{}).get('evidence_count',0)==0,'consensus evidence reported')
    return d

def main(payload):
    node=payload['node']; expected=payload['config'];action=payload['action']
    require(action in ('status','restart','recovery'),'resume action not allowed')
    if action=='restart':require(node['node_id']=='hvm-testnet-v4','only v4 may restart')
    require(os.geteuid()==0 and socket.gethostname()==node['hostname'],'wrong host/root')
    require(hashlib.sha256(Path(BINARY).read_bytes()).hexdigest()=='0148a719eab4a1b956368c75a1af9ceeffb5694986336a8d1b2238b945265dcc','binary mismatch')
    require(json.loads((ETC/'validator.json').read_text())==expected,'validator configuration mismatch')
    pin=(DATA/'runtime.pin').read_bytes()
    require(pin.decode().splitlines()==[DIGEST,expected['node_id'],expected['peer_id'],expected['validator_id']],'identity pin mismatch')
    c=json.loads((ETC/'node.json').read_text())
    observer=dict(expected,role='observer',consensus_key_file='')
    require(c in (observer,expected),'active configuration mismatch')
    require(c==expected,'validator role required')
    if action=='status':
        d=health(expected,'validator')
        rpc=get('/rpc',{'jsonrpc':'2.0','id':1,'method':'hb_getConsensusStatus','params':[]})
        require('error' not in rpc,'consensus RPC error')
        status=rpc['result'];require(status.get('bft_sign_journal_healthy') is True,'journal not healthy')
        require(status.get('next_validator_count')==4,'validator set changed')
        d['consensus']=status;return d
    if action=='restart':
        before=health(expected,'validator')
        path=Path('/root/hvm-testnet-restart-proof.json')
        require(not path.exists(),'restart proof already exists; inspect before repeating')
        run(['systemctl','stop',UNIT])
        raw=(DATA/'consensus-bft-signatures.jsonl').read_bytes()
        records=[json.loads(line) for line in raw.splitlines() if line.strip()]
        require(records,'empty signing journal before restart')
        require(any(r.get('step')=='PRECOMMIT' for r in records),'no precommit before restart')
        marker={'size':len(raw),'sha256':hashlib.sha256(raw).hexdigest(),'max_height':max(r['height'] for r in records),'pin_sha256':hashlib.sha256(pin).hexdigest(),'finalized_before':before['finalized_height']}
        path=Path('/root/hvm-testnet-restart-proof.json')
        require(not path.exists(),'restart proof already exists; inspect before repeating')
        with path.open('x') as f:os.chmod(path,0o600);json.dump(marker,f);f.flush();os.fsync(f.fileno())
        run(['runuser','-u','hashburst-hvm-testnet','--',BINARY,'--config',str(ETC/'node.json'),'--check'])
        run(['systemctl','start',UNIT]);return marker
    if action=='recovery':
        m=json.loads(Path('/root/hvm-testnet-restart-proof.json').read_text())
        raw=(DATA/'consensus-bft-signatures.jsonl').read_bytes()
        require(hashlib.sha256(raw[:m['size']]).hexdigest()==m['sha256'],'journal prefix changed')
        require(hashlib.sha256(pin).hexdigest()==m['pin_sha256'],'pin changed after restart')
        tail=raw[m['size']:].splitlines()
        # A concurrent append can leave an incomplete final line in this snapshot.
        if tail and not raw.endswith(b'\n'):tail=tail[:-1]
        rows=[json.loads(line) for line in tail if line.strip()]
        require(any(r.get('step')=='PRECOMMIT' and r.get('height',0)>m['max_height'] for r in rows),'no new precommit yet')
        return {'journal_prefix_preserved':True,'new_precommit':True,'pin_preserved':True}
    raise RuntimeError('unknown action')
if __name__=='__main__':
    try:
        result=main(PAYLOAD)
        print(json.dumps(result))
    except Exception as exc:
        print('ACTIVATION_ACTION_FAILED: '+str(exc),file=sys.stderr);sys.exit(1)
