#!/usr/bin/env python3
import hashlib,json,os,pwd,shutil,socket,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parent
CONF=Path('/etc/hashburst-hvm-testnet-ingress');DATA=Path('/var/lib/hashburst-hvm-testnet-ingress');BIN=Path('/opt/hashburst-hvm-testnet-ingress');UNIT=Path('/etc/systemd/system/hashburst-hvm-testnet-ingress.service')
def run(*a):return subprocess.check_output(a,text=True).strip()
def main():
 if os.geteuid()!=0:raise RuntimeError('root required')
 addresses=json.loads(run('ip','-j','addr'))
 if not any(i.get('local')=='64.31.4.9' for a in addresses for i in a.get('addr_info',[])):raise RuntimeError('wrong host')
 for line in (ROOT/'SHA256SUMS').read_text().splitlines():
  sha,name=line.split('  ',1)
  if hashlib.sha256((ROOT/name).read_bytes()).hexdigest()!=sha:raise RuntimeError('checksum '+name)
 for p in [CONF,DATA,BIN,UNIT]:
  if p.exists() or p.is_symlink():raise RuntimeError('existing observer installation retained: '+str(p))
 for port in (18009,31307):
  with socket.socket() as s:s.bind(('0.0.0.0',port))
 for host in ['77.90.188.153','77.90.188.154','77.90.188.155','77.90.188.157']:
  with socket.create_connection((host,31307),timeout=8):pass
 user='hashburst-hvm-ingress'
 try:account=pwd.getpwnam(user)
 except KeyError:
  run('useradd','--system','--user-group','--no-create-home','--shell','/usr/sbin/nologin',user);account=pwd.getpwnam(user)
 CONF.mkdir(mode=0o700);DATA.mkdir(mode=0o700);BIN.mkdir(mode=0o755)
 shutil.copyfile(ROOT/'hashburst-testnet',BIN/'hashburst-testnet');(BIN/'hashburst-testnet').chmod(0o755)
 peer=run(str(ROOT/'observer-identity'),'--key',str(CONF/'p2p.key'))
 c=json.loads((ROOT/'template.json').read_text())
 c['bootnodes'].append('/ip4/77.90.188.153/tcp/31307/p2p/'+c['peer_id'])
 c.update(node_id='hvm-testnet-ingress',role='observer',peer_id=peer,validator_id='',consensus_key_file='',data_dir=str(DATA),p2p_key_file=str(CONF/'p2p.key'))
 (CONF/'node.json').write_text(json.dumps(c,indent=2));(CONF/'node.json').chmod(0o600)
 for name in ['blockchain.dat','blockchain.idx']:
  shutil.copyfile(ROOT/'checkpoint'/name,DATA/name);(DATA/name).chmod(0o600)
 for parent in [CONF,DATA]:
  os.chown(parent,account.pw_uid,account.pw_gid)
  for p in parent.iterdir():os.chown(p,account.pw_uid,account.pw_gid)
 run('runuser','-u',user,'--',str(BIN/'hashburst-testnet'),'--config',str(CONF/'node.json'),'--provision')
 UNIT.write_text('''[Unit]
Description=HVM Network testnet non-signing observer
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=hashburst-hvm-ingress
Group=hashburst-hvm-ingress
ExecStart=/opt/hashburst-hvm-testnet-ingress/hashburst-testnet --config /etc/hashburst-hvm-testnet-ingress/node.json
Restart=on-failure
RestartSec=10
TimeoutStopSec=90
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
ReadWritePaths=/var/lib/hashburst-hvm-testnet-ingress
[Install]
WantedBy=multi-user.target
''')
 run('systemctl','daemon-reload');run('systemctl','enable','--now',UNIT.name)
 print('OBSERVER_STARTED_NO_VALIDATOR_SIGNING\nPEER_ID='+peer+'\nRPC=127.0.0.1:18009\nPUBLIC_INGRESS_NOT_YET_ENABLED')
if __name__=='__main__':
 try:main()
 except Exception as e:raise SystemExit('STOP: observer state retained; no automatic rollback. '+str(e))
