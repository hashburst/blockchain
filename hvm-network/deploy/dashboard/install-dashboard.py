#!/usr/bin/env python3
"""Install HVM dashboard V5 without modifying HVM runtime or keys."""
import hashlib,json,os,shutil,subprocess,tempfile,time,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
PAGE=Path('/var/www/blockchainapi.one/public/hashburst/index.php')
VHOST=Path('/etc/nginx/sites-available/blockchainapi.one.conf')
SNIPPET=Path('/etc/nginx/snippets/hvm-network-status.conf')
def patch(text):
 if 'HVM_NETWORK_CARD_V5_EN' in text:return text
 if 'HVM_NETWORK_CARD_V4_EN' in text:
  for name in ('card.html','card.js'):
   old=(R/'dashboard'/('v4-'+name)).read_text()
   if text.count(old)!=1:raise RuntimeError('existing V4 panel differs; retained')
   text=text.replace(old,(R/'dashboard'/name).read_text(),1)
  return text
 if 'id="nav-hvm"' in text:raise RuntimeError('existing HVM view requires exact V4 upgrade; no overwrite')
 nav='    <button data-view="storage" id="nav-storage">Sovereign Storage</button>';js="\nloadView('explorer');\n"
 for anchor in (nav,'</main>',js):
  if text.count(anchor)!=1:raise RuntimeError('dashboard source differs at '+repr(anchor))
 text=text.replace(nav,nav+'\n    <button data-view="hvm" id="nav-hvm">HVM Network</button>')
 text=text.replace('</main>',(R/'dashboard/card.html').read_text()+'\n</main>')
 return text.replace(js,'\n'+(R/'dashboard/card.js').read_text()+js)
def run(*args):return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True,timeout=25)
def get_local(path):
 return run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','https://blockchainapi.one'+path)
def main():
 if os.geteuid()!=0:raise RuntimeError('run on 64.31.4.9 as root')
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open('http://127.0.0.1:18019/health',timeout=15) as f:d=json.load(f)
 if d.get('chain_id')!=4735489 or d.get('network')!='mainnet' or d.get('role')!='observer' or d.get('node_id')!='hvm-mainnet-observer-01':raise RuntimeError('wrong mainnet observer')
 if Path('/etc/nginx/sites-enabled/blockchainapi.one.conf').resolve()!=VHOST:raise RuntimeError('unexpected enabled vhost')
 original=PAGE.read_text();updated=patch(original);v=VHOST.read_text()
 anchor='    include /etc/nginx/snippets/hvm-testnet-routes.conf;';include='    include /etc/nginx/snippets/hvm-network-status.conf;'
 if include not in v:
  if v.count(anchor)!=1:raise RuntimeError('vhost anchor differs')
  newv=v.replace(anchor,anchor+'\n'+include)
 else:newv=v
 snippet=(R/'dashboard/routes.conf').read_text()
 backup=Path(tempfile.mkdtemp(prefix='hvm-dashboard-v5-',dir='/root'))
 shutil.copy2(PAGE,backup/'index.php');shutil.copy2(VHOST,backup/'vhost.conf');had=SNIPPET.exists()
 if had:shutil.copy2(SNIPPET,backup/'snippet.conf')
 print('BACKUP='+str(backup),flush=True)
 try:
  PAGE.write_text(updated);SNIPPET.write_text(snippet);VHOST.write_text(newv)
  run('php','-l',str(PAGE));run('nginx','-t');run('systemctl','reload','nginx')
  for attempt in range(15):
   try:
    n=json.loads(get_local('/api/hashburst/hvm/mainnet/network'))
    if n.get('chain_id')!=4735489 or n.get('role')!='observer' or n.get('node_id')!='hvm-mainnet-observer-01':raise RuntimeError('mainnet status mismatch')
    rpc=json.loads(run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','-H','Content-Type: application/json','--data','{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}','https://blockchainapi.one/api/hashburst/hvm/mainnet/evm'))
    if rpc.get('result')!='0x484201':raise RuntimeError('mainnet RPC mismatch')
    page=get_local('/hashburst/')
    if 'HVM_NETWORK_CARD_V5_EN' not in page or 'id="nav-hvm"' not in page:raise RuntimeError('dashboard not updated')
    print('HVM_DASHBOARD_V5_LOCAL_VERIFIED');return
   except Exception:
    if attempt==14:raise
    time.sleep(1)
 except BaseException:
  shutil.copy2(backup/'index.php',PAGE);shutil.copy2(backup/'vhost.conf',VHOST)
  if had:shutil.copy2(backup/'snippet.conf',SNIPPET)
  else:SNIPPET.unlink(missing_ok=True)
  run('nginx','-t');run('systemctl','reload','nginx');raise
if __name__=='__main__':main()
