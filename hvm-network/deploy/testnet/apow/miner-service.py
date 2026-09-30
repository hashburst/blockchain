#!/usr/bin/env python3
"""Stage an isolated testnet miner. Does not migrate or restart a blockchain node."""
import argparse,hashlib,json,os,pwd,shutil,subprocess,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
BASE=Path('/opt/hashburst-apow-miner')
KEYDIR=Path('/var/lib/hashburst-apow-miner')
UNIT=Path('/etc/systemd/system/hashburst-apow-miner.service')
USER='hashburst-apow-miner'
def run(*args):return subprocess.run(args,check=True,capture_output=True,text=True).stdout.strip()
def health():
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
 with opener.open('http://127.0.0.1:18009/health',timeout=15) as r:return json.load(r)
def validate(c,h):
 if c.get('network')!='testnet' or c.get('protocol',{}).get('chain_id')!=4735490 or h.get('chain_id')!=4735490:raise ValueError('testnet 4735490 only')
 if c.get('role')!='validator' or h.get('role')!='validator' or c.get('node_id')!=h.get('node_id'):raise ValueError('matching validator identity required')
def main():
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=['stage','start']);a=parser.parse_args()
 if os.geteuid()!=0:raise ValueError('run as root on a testnet validator')
 c=json.loads(Path('/etc/hashburst-hvm-testnet/node.json').read_text());validate(c,health())
 release=json.loads((R/'miner-release.json').read_text());binary=BASE/release['sha256']/'hvm-apow-miner'
 if a.action=='start':
  if not c['protocol'].get('apow'):raise ValueError('APoW consensus configuration not installed; no service started')
  if not binary.is_file() or hashlib.sha256(binary.read_bytes()).hexdigest()!=release['sha256']:raise ValueError('staged binary mismatch')
  if UNIT.read_text()!=unit(binary):raise ValueError('miner unit mismatch')
  run('systemctl','enable','--now',UNIT.name);print('TESTNET_MINER_STARTED_REWARD_REQUIRES_FINALITY');return
 source=R/'hvm-apow-miner'
 if hashlib.sha256(source.read_bytes()).hexdigest()!=release['sha256']:raise ValueError('binary digest mismatch')
 if UNIT.exists() and UNIT.read_text()!=unit(binary):raise ValueError('existing miner unit differs; retained')
 for p in (BASE,KEYDIR):
  if p.is_symlink():raise ValueError('symlinked deployment path')
 try:account=pwd.getpwnam(USER)
 except KeyError:
  run('useradd','--system','--home-dir',str(KEYDIR),'--shell','/usr/sbin/nologin',USER);account=pwd.getpwnam(USER)
 BASE.mkdir(mode=0o755,exist_ok=True);binary.parent.mkdir(mode=0o755,exist_ok=True)
 if binary.exists():
  if binary.is_symlink() or hashlib.sha256(binary.read_bytes()).hexdigest()!=release['sha256']:raise ValueError('existing release differs')
 else:shutil.copyfile(source,binary);binary.chmod(0o755)
 KEYDIR.mkdir(mode=0o700,exist_ok=True);os.chown(KEYDIR,account.pw_uid,account.pw_gid);KEYDIR.chmod(0o700)
 key=KEYDIR/'miner.key'
 if key.is_symlink():raise ValueError('symlinked miner key')
 if key.exists():
  info=key.stat()
  if not key.is_file() or info.st_uid!=account.pw_uid or info.st_mode & 0o077:raise ValueError('miner key permissions/owner mismatch')
  print(run('runuser','-u',USER,'--',str(binary),'--key-file',str(key),'--identity-only'))
 else:print(run('runuser','-u',USER,'--',str(binary),'--key-file',str(key),'--generate-key'))
 if not UNIT.exists():
  with UNIT.open('x') as f:f.write(unit(binary))
 UNIT.chmod(0o644);run('systemd-analyze','verify',str(UNIT));run('systemctl','daemon-reload')
 print('MINER_STAGED_NO_SERVICE_STARTED_NO_NODE_CONFIG_CHANGED')
def unit(binary):
 return f'''[Unit]
Description=HVM Network testnet APoW miner
After=hashburst-hvm-testnet.service
[Service]
Type=simple
User={USER}
Group={USER}
ExecStart={binary} --key-file {KEYDIR}/miner.key --chain-id 4735490 --loop --timeout 10s
Restart=on-failure
RestartSec=5
Nice=15
CPUQuota=25%
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
[Install]
WantedBy=multi-user.target
'''
if __name__=='__main__':
 try:main()
 except Exception as e:raise SystemExit('STOP: '+str(e)+'; existing state retained')
