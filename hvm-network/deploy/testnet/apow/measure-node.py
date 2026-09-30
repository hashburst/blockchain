"""Measure the staged miner against isolated loopback challenges, not chain jobs."""
import hashlib,json,os,secrets,struct,subprocess,threading,time,re
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
MINER_SHA='32e62dc2a41d16c0ea472f5aa69e53f0a752cc13c1c9ca61c5e25af7a9998cdc'
RUNTIME_SHA='0e597329261f9376d9c33b1fe427638fd2810bfd43ce5bd702451e785d9f339d'
MINER=Path('/opt/hashburst-apow-miner')/MINER_SHA/'hvm-apow-miner'
KEY=Path('/var/lib/hashburst-apow-miner/miner.key')
def proof_digest(p):
 def u(n):return struct.pack('>Q',n)
 def s(t):b=t.encode();return u(len(b))+b
 raw=s('HASHBURST_APOW_SHA256_V1')+u(p['chain_id'])+u(p['height'])+s(p['parent_hash'])+struct.pack('>q',p['poh'])+u(p['bits'])+struct.pack('>q',p['epoch_start'])+s(p['author'])+s(p['beneficiary'])+u(p['nonce'])
 return hashlib.sha256(raw).digest()
def check_proof(p,job,address):
 for k in ('chain_id','height','parent_hash','poh','bits','epoch_start'):
  if p.get(k)!=job[k]:raise ValueError('synthetic job changed: '+k)
 if p.get('author')!=address.lower() or p.get('beneficiary')!=address.lower():raise ValueError('miner identity/beneficiary mismatch')
 if type(p.get('nonce')) is not int or not 0<=p['nonce']<2**64:raise ValueError('invalid nonce')
 if not isinstance(p.get('signature'),str) or not re.fullmatch('[0-9a-f]{130}',p['signature']):raise ValueError('invalid signature encoding')
 h=proof_digest(p)
 if int.from_bytes(h,'big') >= 1 << (256-job['bits']):raise ValueError('work target not met')
 return h.hex()
class ChallengeServer(ThreadingHTTPServer):
 daemon_threads=True
 def __init__(self,job,address):
  super().__init__(('127.0.0.1',0),ChallengeHandler)
  self.job=job;self.address=address;self.token='/'+secrets.token_hex(24);self.started=None;self.proof=None;self.elapsed=None;self.failure=None
class ChallengeHandler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def reply(self,status,obj):
  raw=json.dumps(obj).encode();self.send_response(status);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
 def do_GET(self):
  if self.path!=self.server.token:return self.reply(404,{'error':'path'})
  self.server.started=time.monotonic();self.reply(200,self.server.job)
 def do_POST(self):
  try:
   if self.path!=self.server.token or self.server.started is None or self.server.proof is not None:raise ValueError('unexpected submission')
   size=int(self.headers.get('Content-Length','0'))
   if not 0<size<=2048:raise ValueError('invalid size')
   self.connection.settimeout(5);proof=json.loads(self.rfile.read(size))
   check_proof(proof,self.server.job,self.server.address)
   self.server.elapsed=time.monotonic()-self.server.started;self.server.proof=proof
   self.reply(200,{'ok':True,'synthetic_only':True})
  except Exception as e:
   self.server.failure=str(e);self.reply(400,{'error':str(e)})
def command(unit,endpoint):
 return ['systemd-run','--quiet','--wait','--pipe','--collect','--unit='+unit,
 '--property=User=hashburst-apow-miner','--property=Group=hashburst-apow-miner',
 '--property=CPUQuota=25%','--property=Nice=15','--property=RuntimeMaxSec=20',
 '--property=KillMode=control-group','--property=NoNewPrivileges=yes',
 '--property=ProtectSystem=strict','--property=ProtectHome=yes','--property=PrivateTmp=yes',
 '--property=IPAddressDeny=any','--property=IPAddressAllow=localhost',
 str(MINER),'--key-file',str(KEY),'--chain-id','4735490','--endpoint',endpoint,'--timeout','10s']
def measure(request):
 node=request['node_id']
 if node not in ('hvm-testnet-v1','hvm-testnet-v2','hvm-testnet-v3','hvm-testnet-v4'):raise ValueError('four testnet validators only')
 before=inspect({'action':'status','node_id':node})
 if before['protocol'].get('apow'):raise ValueError('APoW already configured')
 if MINER.is_symlink() or digest(MINER)!=MINER_SHA:raise ValueError('staged miner mismatch')
 runtime=Path('/opt/hashburst-apow-candidate')/RUNTIME_SHA/'hashburst-testnet'
 if runtime.is_symlink() or digest(runtime)!=RUNTIME_SHA:raise ValueError('staged runtime mismatch')
 cfg=json.loads(Path('/etc/hashburst-hvm-testnet/node.json').read_text());protected_before=protected(cfg)
 state=subprocess.run(['systemctl','is-active','hashburst-apow-miner.service'],capture_output=True,text=True,timeout=10).stdout.strip()
 if state in ('active','activating','reloading','deactivating'):raise ValueError('production miner not stopped')
 identity=subprocess.check_output(['runuser','-u','hashburst-apow-miner','--',str(MINER),'--key-file',str(KEY),'--identity-only'],text=True,timeout=10).strip()
 match=re.fullmatch('APOW_MINER_ADDRESS=(0x[0-9a-fA-F]{40})',identity)
 if not match:raise ValueError('unexpected miner identity')
 address=match[1];samples=[]
 for bits in (14,16,18,18,18,18):
  # A random parent with height 1 is not a job from the live chain.
  job={'chain_id':4735490,'height':1,'parent_hash':secrets.token_hex(32),'poh':0,'bits':bits,'epoch_start':0,'nonce':0}
  server=ChallengeServer(job,address);thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
  unit='hvm-apow-measure-'+secrets.token_hex(8)
  try:
   r=subprocess.run(command(unit,'http://127.0.0.1:'+str(server.server_port)+server.token),capture_output=True,text=True,timeout=30)
   if r.returncode==0 and server.proof is not None:
    attempts=server.proof['nonce']+1;elapsed=server.elapsed
    samples.append({'bits':bits,'attempts':attempts,'elapsed_seconds':elapsed,'hashes_per_second':attempts/elapsed,'status':'success','proof':server.proof})
   elif 'context deadline exceeded' in r.stderr and server.failure is None:
    samples.append({'bits':bits,'status':'timeout','limit_seconds':10})
   else:raise ValueError('measurement failed: '+(server.failure or (r.stdout+r.stderr)[-1800:]))
  finally:
   # Only this random transient measurement unit can be stopped here.
   subprocess.run(['systemctl','stop',unit+'.service'],capture_output=True,text=True,timeout=15)
   server.shutdown();server.server_close();thread.join(timeout=2)
 verify_protected(protected_before)
 after=inspect({'action':'status','node_id':node})
 if before['pin_sha256']!=after['pin_sha256'] or before['binary_sha256']!=after['binary_sha256']:raise ValueError('node pin/binary changed')
 if after['height']<=before['height']:raise ValueError('finality did not advance during measurement')
 success=[s for s in samples if s['status']=='success']
 if not success:raise ValueError('no successful synthetic work sample')
 return {'node_id':node,'address':address,'miner_sha256':MINER_SHA,'cpu_quota_percent':25,'nice':15,'samples':samples,'measured_hashes_per_second':sum(s['attempts'] for s in success)/sum(s['elapsed_seconds'] for s in success),'height_before':before['height'],'height_after':after['height'],'protected_prefixes':protected_before,'synthetic_only':True,'no_live_work_submitted':True,'no_node_restart':True,'apow_activated':False,'signature_verification':'staged Go miner self-verifies; collector checks digest, target and identity, not ECDSA independently'}
def main(request):
 if request['action']=='measure':return measure(request)
 return inspect(request)
