#!/usr/bin/env python3
"""Install EVM routes only after observer EVM finality. Native ingress retained."""
import hashlib,json,os,shutil,subprocess,tempfile,time,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
V=Path('/etc/nginx/sites-available/blockchainapi.one.conf');S=Path('/etc/nginx/snippets/hvm-testnet-evm.conf');APP=Path('/opt/hashburst-hvm-evm-gateway');U=Path('/etc/systemd/system/hashburst-hvm-evm-gateway.service')
def run(*args):return subprocess.check_output(args,text=True,stderr=subprocess.STDOUT).strip()
def rpc(url,method,params=[]):
 req=urllib.request.Request(url,json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(),{'Content-Type':'application/json'})
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=20) as r:d=json.load(r)
 if 'error' in d:raise RuntimeError(str(d['error']))
 return d['result']
def main():
 if os.geteuid()!=0:raise RuntimeError('root required')
 c=json.loads(Path('/etc/hashburst-hvm-testnet-ingress/node.json').read_text())
 if c['role']!='observer' or c.get('consensus_key_file') or c['protocol']['chain_id']!=4735490 or not c['protocol'].get('evm'):raise RuntimeError('activated unsigned observer required')
 for name in ('consensus-votes.jsonl','consensus-bft-signatures.jsonl'):
  if (Path(c['data_dir'])/name).stat().st_size:raise RuntimeError('observer signed')
 url='http://127.0.0.1:18009/evm'
 if rpc(url,'eth_chainId')!='0x484202':raise RuntimeError('chain ID mismatch')
 block=rpc(url,'eth_getBlockByNumber',['finalized',False])
 if int(block['number'],16)<c['protocol']['evm']['activation_height']:raise RuntimeError('EVM activation not finalized')
 for line in (R/'SHA256SUMS').read_text().splitlines():
  sha,name=line.split('  ',1)
  if hashlib.sha256((R/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('checksum '+name)
 if Path('/etc/nginx/sites-enabled/blockchainapi.one.conf').resolve()!=V:raise RuntimeError('unexpected vhost link')
 anchor='    include /etc/nginx/snippets/hvm-testnet-routes.conf;'
 text=V.read_text()
 if text.count(anchor)!=1 or 'hvm-testnet-evm.conf' in text or S.exists() or U.exists() or APP.exists():raise RuntimeError('unexpected or existing ingress; retain and inspect')
 backup=Path(tempfile.mkdtemp(prefix='hvm-evm-ingress-',dir='/root'));shutil.copy2(V,backup/'vhost.conf');print('BACKUP='+str(backup),flush=True)
 APP.mkdir(mode=0o755);APP.chmod(0o755);shutil.copy2(R/'hvm-evm-gateway',APP/'hvm-evm-gateway');(APP/'hvm-evm-gateway').chmod(0o755);shutil.copy2(R/'metamask-canary.html',APP/'metamask-canary.html');(APP/'metamask-canary.html').chmod(0o644)
 U.write_text('[Unit]\nDescription=HVM Network public EVM testnet gateway\nAfter=network.target hashburst-hvm-testnet-ingress.service\n[Service]\nDynamicUser=yes\nExecStart=/opt/hashburst-hvm-evm-gateway/hvm-evm-gateway\nRestart=on-failure\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nRestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX\nMemoryMax=256M\nTasksMax=64\n[Install]\nWantedBy=multi-user.target\n')
 U.chmod(0o644)
 try:
  run('systemctl','daemon-reload');run('systemctl','enable','--now',U.name)
  for i in range(20):
   try:
    if rpc('http://127.0.0.1:18011/rpc','eth_chainId')=='0x484202':break
   except Exception:
    if i==19:raise
    time.sleep(1)
  shutil.copy2(R/'routes.conf',S);S.chmod(0o644);V.write_text(text.replace(anchor,anchor+'\n    include /etc/nginx/snippets/hvm-testnet-evm.conf;',1))
  run('nginx','-t');run('systemctl','reload','nginx')
  for i in range(20):
   try:
    d=json.loads(run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','-H','Content-Type: application/json','--data','{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}','https://blockchainapi.one/api/hashburst/hvm/testnet/evm'))
    if d.get('result')!='0x484202':raise RuntimeError('wrong public response')
    break
   except Exception:
    if i==19:raise
    time.sleep(1)
  print('EVM_INGRESS_INSTALLED_RUN_PUBLIC_AND_METAMASK_TESTS')
 except BaseException:
  V.write_text(text);S.unlink(missing_ok=True);run('nginx','-t');run('systemctl','reload','nginx');subprocess.run(['systemctl','disable','--now',U.name]);raise
if __name__=='__main__':main()

