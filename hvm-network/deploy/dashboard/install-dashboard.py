#!/usr/bin/env python3
"""Install HVM dashboard V5.2 without modifying HVM runtime or keys."""
import hashlib,json,os,shutil,subprocess,tempfile,time,urllib.request
from pathlib import Path
R=Path(__file__).resolve().parent
PAGE=Path('/var/www/blockchainapi.one/public/hashburst/index.php')
VHOST=Path('/etc/nginx/sites-available/blockchainapi.one.conf')
SNIPPET=Path('/etc/nginx/snippets/hvm-network-status.conf')
LIVE_V3_SHA256='18cfc5a7e4bf35aff5889c0f3a84e1a2cdb61169c36c34a442b65c9cfd712a57'
def patch_ecosystem(text):
 oldm="['HVM mainnet observer', '/api/hashburst/hvm/mainnet/health', d => d.chain_id === 4735489 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)]"
 newm="['HVM mainnet observer', '/api/hashburst/hvm/mainnet/network', d => d.ok === true && d.network === 'mainnet' && d.chain_id === 4735489 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)]"
 oldt="['HVM testnet observer', '/api/hashburst/hvm/testnet/health', d => d.chain_id === 4735490 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)]"
 newt="['HVM testnet observer', '/api/hashburst/hvm/testnet/network', d => d.ok === true && d.network === 'testnet' && d.chain_id === 4735490 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)]"
 for old,new in ((oldm,newm),(oldt,newt)):
  if old in text:text=text.replace(old,new,1)
  elif new not in text:raise RuntimeError('ecosystem HVM probe guard differs; retained')
 return text
def patch_v3(text):
 hs=text.index('<div class="view" id="view-hvm">');he=text.find('\n</main>',hs)
 js=text.index('// HVM_NETWORK_CARD_V3');je=text.find("\nif (location.hash && document.getElementById('nav-' + location.hash.slice(1)))",js)
 if he<0 or je<0:raise RuntimeError('V3 HVM boundary missing; retained')
 old_html=text[hs:he];old_js=text[js:je]
 for g in ('HashBurst HVM Testnet','4735490 (0x484202)','id="hvm-peers"','4735489: under maintenance'):
  if g not in old_html:raise RuntimeError('V3 HVM HTML guard differs: '+g)
 if "/api/hashburst/hvm/testnet/network" not in old_js or "d.chain_id !== 4735490" not in old_js:raise RuntimeError('V3 HVM JS guard differs; retained')
 return text[:hs]+(R/'dashboard/card.html').read_text().lstrip()+text[he:js]+(R/'dashboard/card.js').read_text().rstrip()+text[je:]
def patch_v5(text):
 hs=text.index('<div class="view" id="view-hvm">');he=text.find('\n</main>',hs)
 js=text.index('// HVM_NETWORK_CARD_V5_EN');je=text.find("\nif (location.hash && document.getElementById('nav-' + location.hash.slice(1)))",js)
 if he<0 or je<0:raise RuntimeError('V5 HVM boundary missing; retained')
 old_html=text[hs:he];old_js=text[js:je]
 for g in ('Live read-only mainnet observer status','4735489 (0x484201)','id="hvm-refresh"','Validator federation'):
  if g not in old_html:raise RuntimeError('V5 HVM HTML guard differs: '+g)
 if "/api/hashburst/hvm/mainnet/network" not in old_js or "d.chain_id!==4735489" not in old_js:raise RuntimeError('V5 HVM JS guard differs; retained')
 return text[:hs]+(R/'dashboard/card.html').read_text().lstrip()+text[he:js]+(R/'dashboard/card.js').read_text().rstrip()+text[je:]
def patch(text):
 if 'HVM_NETWORK_CARD_V5_2_EN' in text:return patch_ecosystem(text)
 if 'HVM_NETWORK_CARD_V5_EN' in text:return patch_ecosystem(patch_v5(text))
 if 'HVM_NETWORK_CARD_V4_EN' in text:
  for name in ('card.html','card.js'):
   old=(R/'dashboard'/('v4-'+name)).read_text()
   if text.count(old)!=1:raise RuntimeError('existing V4 panel differs; retained')
   text=text.replace(old,(R/'dashboard'/name).read_text(),1)
  return patch_ecosystem(text)
 if 'HVM_NETWORK_CARD_V3' in text:return patch_ecosystem(patch_v3(text))
 if 'id="nav-hvm"' in text:raise RuntimeError('existing HVM view is unknown; no overwrite')
 nav='    <button data-view="storage" id="nav-storage">Sovereign Storage</button>';js="\nloadView('explorer');\n"
 for anchor in (nav,'</main>',js):
  if text.count(anchor)!=1:raise RuntimeError('dashboard source differs at '+repr(anchor))
 text=text.replace(nav,nav+'\n    <button data-view="hvm" id="nav-hvm">HVM Network</button>')
 text=text.replace('</main>',(R/'dashboard/card.html').read_text()+'\n</main>')
 return patch_ecosystem(text.replace(js,'\n'+(R/'dashboard/card.js').read_text()+js))
def run(*args):return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True,timeout=25)
def get_local(path):
 return run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','https://blockchainapi.one'+path)
def main():
 if os.geteuid()!=0:raise RuntimeError('run on 64.31.4.9 as root')
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
 expected=(('http://127.0.0.1:18019/health',4735489,'mainnet','hvm-mainnet-observer-01'),('http://127.0.0.1:18009/health',4735490,'testnet','hvm-testnet-ingress'))
 for url,chain,network,node in expected:
  with opener.open(url,timeout=15) as f:d=json.load(f)
  if d.get('chain_id')!=chain or d.get('network')!=network or d.get('role')!='observer' or d.get('node_id')!=node:raise RuntimeError('wrong '+network+' observer')
 if Path('/etc/nginx/sites-enabled/blockchainapi.one.conf').resolve()!=VHOST:raise RuntimeError('unexpected enabled vhost')
 original=PAGE.read_text()
 if 'HVM_NETWORK_CARD_V3' in original:
  actual=hashlib.sha256(original.encode()).hexdigest()
  if actual!=LIVE_V3_SHA256:raise RuntimeError('live V3 page hash differs; retained: '+actual)
 updated=patch(original);v=VHOST.read_text()
 anchor='    include /etc/nginx/snippets/hvm-testnet-routes.conf;';include='    include /etc/nginx/snippets/hvm-network-status.conf;'
 if include not in v:
  if v.count(anchor)!=1:raise RuntimeError('vhost anchor differs')
  newv=v.replace(anchor,anchor+'\n'+include)
 else:newv=v
 snippet=(R/'dashboard/routes.conf').read_text()
 backup=Path(tempfile.mkdtemp(prefix='hvm-dashboard-v5-2-',dir='/root'))
 shutil.copy2(PAGE,backup/'index.php');shutil.copy2(VHOST,backup/'vhost.conf');had=SNIPPET.exists()
 if had:shutil.copy2(SNIPPET,backup/'snippet.conf')
 print('BACKUP='+str(backup),flush=True)
 try:
  PAGE.write_text(updated);SNIPPET.write_text(snippet);VHOST.write_text(newv)
  run('php','-l',str(PAGE));run('nginx','-t');run('systemctl','reload','nginx')
  for attempt in range(15):
   try:
    for network,chain,node in (('mainnet',4735489,'hvm-mainnet-observer-01'),('testnet',4735490,'hvm-testnet-ingress')):
     n=json.loads(get_local('/api/hashburst/hvm/'+network+'/network'))
     if n.get('ok') is not True or n.get('network')!=network or n.get('chain_id')!=chain or n.get('role')!='observer' or n.get('node_id')!=node:raise RuntimeError(network+' status mismatch')
    for network,hexid in (('mainnet','0x484201'),('testnet','0x484202')):
     rpc=json.loads(run('curl','--noproxy','*','--resolve','blockchainapi.one:443:127.0.0.1','-fsS','--max-time','20','-H','Content-Type: application/json','--data','{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}','https://blockchainapi.one/api/hashburst/hvm/'+network+'/evm'))
     if rpc.get('result')!=hexid:raise RuntimeError(network+' RPC mismatch')
    page=get_local('/hashburst/')
    for marker in ('HVM_NETWORK_CARD_V5_2_EN','id="hvm-connect-mainnet"','id="hvm-connect-testnet"','/api/hashburst/hvm/mainnet/network','/api/hashburst/hvm/testnet/network'):
     if marker not in page:raise RuntimeError('dashboard missing '+marker)
    print('HVM_DASHBOARD_V5_2_DUAL_NETWORK_LOCAL_VERIFIED');return
   except Exception:
    if attempt==14:raise
    time.sleep(1)
 except BaseException:
  shutil.copy2(backup/'index.php',PAGE);shutil.copy2(backup/'vhost.conf',VHOST)
  if had:shutil.copy2(backup/'snippet.conf',SNIPPET)
  else:SNIPPET.unlink(missing_ok=True)
  run('nginx','-t');run('systemctl','reload','nginx');raise
if __name__=='__main__':main()
