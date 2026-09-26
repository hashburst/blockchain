#!/usr/bin/env python3
import hashlib,json,os,shutil,subprocess,tempfile,time,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
V=Path('/etc/nginx/sites-available/blockchainapi.one.conf')
Z=Path('/etc/nginx/conf.d/hvm-testnet-limits.conf')
S=Path('/etc/nginx/snippets/hvm-testnet-routes.conf')
APP=Path('/opt/hashburst-hvm-public-gateway')
U=Path('/etc/systemd/system/hashburst-hvm-public-gateway.service')
def run(*a):return subprocess.check_output(a,text=True).strip()
def main():
 if os.geteuid()!=0:raise RuntimeError('root required')
 for line in (R/'SHA256SUMS').read_text().splitlines():
  sha,name=line.split('  ',1)
  if hashlib.sha256((R/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('checksum '+name)
 c=json.loads(Path('/etc/hashburst-hvm-testnet-ingress/node.json').read_text())
 if c['peer_id']!='12D3KooWDdtS7twaerqrivLp2Gp4CBWSg1H35NqkAp6YoYy7e8ed' or c['role']!='observer' or c['consensus_key_file'] or c['protocol']['chain_id']!=4735490:raise RuntimeError('unexpected observer')
 for name in ['consensus-votes.jsonl','consensus-bft-signatures.jsonl']:
  if (Path(c['data_dir'])/name).stat().st_size:raise RuntimeError('nonempty signing journal')
 run('systemctl','is-active','hashburst-hvm-testnet-ingress.service')
 if Path('/etc/nginx/sites-enabled/blockchainapi.one.conf').resolve()!=V:raise RuntimeError('vhost target differs')
 text=V.read_text();anchor='    location /api/hashburst/ {\n'
 if text.count(anchor)!=1 or '/api/hashburst/hvm/testnet/' in text:raise RuntimeError('vhost anchor differs or routes already present')
 for p in [Z,S,APP,U]:
  if p.exists() or p.is_symlink():raise RuntimeError('existing deployment retained: '+str(p))
 backup=Path(tempfile.mkdtemp(prefix='hvm-public-ingress-',dir='/root'));shutil.copy2(V,backup/'vhost.conf')
 print('BACKUP='+str(backup),flush=True)
 APP.mkdir(mode=0o755);shutil.copyfile(R/'gateway.py',APP/'gateway.py');(APP/'gateway.py').chmod(0o644)
 U.write_text('''[Unit]
Description=HVM Network filtered testnet public RPC gateway
After=network.target hashburst-hvm-testnet-ingress.service
[Service]
DynamicUser=yes
ExecStart=/usr/bin/python3 /opt/hashburst-hvm-public-gateway/gateway.py
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
[Install]
WantedBy=multi-user.target
''')
 changed=False
 try:
  run('systemctl','daemon-reload');run('systemctl','enable','--now',U.name)
  opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
  for attempt in range(20):
   try:
    with opener.open('http://127.0.0.1:18010/health',timeout=15) as response:d=json.load(response)
    if d['chain_id']==4735490:break
   except Exception:
    if attempt==19:raise
    time.sleep(2)
  shutil.copyfile(R/'hvm-testnet-limits.conf',Z);shutil.copyfile(R/'routes.conf',S)
  changed=True;V.write_text(text.replace(anchor,'    include /etc/nginx/snippets/hvm-testnet-routes.conf;\n\n'+anchor,1))
  run('nginx','-t');run('systemctl','reload','nginx')
  for attempt in range(20):
   try:
    raw=run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','https://blockchainapi.one/api/hashburst/hvm/testnet/health')
    if json.loads(raw)['chain_id']!=4735490:raise RuntimeError('wrong health')
    break
   except Exception:
    if attempt==19:raise
    time.sleep(2)
  print('HVM_TESTNET_HTTPS_INGRESS_INSTALLED\nRUN_PUBLIC_VERIFY_FROM_MAC')
 except BaseException:
  if changed:
   shutil.copyfile(backup/'vhost.conf',V)
   Z.unlink(missing_ok=True);S.unlink(missing_ok=True)
   run('nginx','-t');run('systemctl','reload','nginx')
  subprocess.run(['systemctl','disable','--now',U.name],check=False)
  print('INGRESS_FAILED: observer unchanged; inspect backup '+str(backup));raise
if __name__=='__main__':main()
