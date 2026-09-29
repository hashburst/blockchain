#!/usr/bin/env python3
"""Install the public EVM ingress after the five-node and restart gates."""
import hashlib,json,subprocess,tarfile,tempfile
from pathlib import Path
R=Path(__file__).resolve().parent
GATE=R/'GATE-restart-v4.json'
def main():
 gate=json.loads(GATE.read_text());sha=hashlib.sha256((R/'hashburst-testnet').read_bytes()).hexdigest()
 if not gate.get('ok') or gate.get('chain_id')!=4735490 or gate.get('binary_sha256')!=sha or gate.get('height',0)<53303:raise RuntimeError('restart gate for this binary required')
 names=['install-ingress.py','hvm-evm-gateway','routes.conf','metamask-canary.html']
 with tempfile.TemporaryDirectory() as tmp:
  p=Path(tmp);manifest=''
  for name in names:
   raw=(R/name).read_bytes();(p/name).write_bytes(raw);manifest+=hashlib.sha256(raw).hexdigest()+'  '+name+'\n'
  (p/'SHA256SUMS').write_text(manifest)
  archive=p/'ingress.tar.gz'
  with tarfile.open(archive,'w:gz') as tar:
   for name in names+['SHA256SUMS']:tar.add(p/name,arcname=name)
  dest='/root/hvm-evm-ingress-'+sha[:12]
  subprocess.run(['scp',str(archive),'root@64.31.4.9:/root/hvm-evm-ingress-repair.tar.gz'],check=True)
  command='mkdir -p '+dest+' && chmod 700 '+dest+' && tar -xzf /root/hvm-evm-ingress-repair.tar.gz -C '+dest+' && cd '+dest+' && sha256sum -c SHA256SUMS && python3 install-ingress.py'
  subprocess.run(['ssh','-T','-o','ControlMaster=no','-o','ControlPath=none','root@64.31.4.9',command],check=True)
 subprocess.run(['python3',str(R/'verify-public.py')],check=True,cwd=R)
 print('EVM_PUBLIC_INGRESS_READY_FOR_WALLET_TEST')
if __name__=='__main__':main()
