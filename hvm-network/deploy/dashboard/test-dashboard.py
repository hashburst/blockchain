import importlib.util,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('installer',R/'install-dashboard.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class PatchTests(unittest.TestCase):
 def source(self):return '<nav>\n    <button data-view="storage" id="nav-storage">Sovereign Storage</button>\n</nav><main></main><script nonce="preserved">\nloadView(\'explorer\');\n</script>'
 def ecosystem(self,s):
  return s.replace("loadView('explorer');","const checks = [\n    ['HVM mainnet observer', '/api/hashburst/hvm/mainnet/health', d => d.chain_id === 4735489 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)],\n    ['HVM testnet observer', '/api/hashburst/hvm/testnet/health', d => d.chain_id === 4735490 && d.role === 'observer' && Number.isSafeInteger(d.finalized_height)]\n  ];\nloadView('explorer');")
 def test_install_idempotence(self):
  s=m.patch(self.source());self.assertEqual(s,m.patch(s));self.assertIn('HVM_NETWORK_CARD_V5_2_EN',s);self.assertIn('nonce="preserved"',s)
 def test_exact_v4_upgrade(self):
  s=m.patch(self.source())
  for name in ('card.html','card.js'):s=s.replace((R/'dashboard'/name).read_text(),(R/'dashboard'/('v4-'+name)).read_text())
  self.assertIn('HVM_NETWORK_CARD_V5_2_EN',m.patch(s))
 def test_guarded_v3_upgrade(self):
  h='<div class="view" id="view-hvm">HashBurst HVM Testnet 4735490 (0x484202)<span id="hvm-peers"></span>4735489: under maintenance</div>'
  j="// HVM_NETWORK_CARD_V3\nfetch('/api/hashburst/hvm/testnet/network'); if (d.chain_id !== 4735490) throw Error();"
  s='PREFIX\n'+h+'\n</main>\nKEEP\n'+j+"\nif (location.hash && document.getElementById('nav-' + location.hash.slice(1))) KEEP();\nSUFFIX"
  out=m.patch(s);self.assertIn('HVM_NETWORK_CARD_V5_2_EN',out);self.assertNotIn('HVM_NETWORK_CARD_V3',out)
  with self.assertRaises(RuntimeError):m.patch(s.replace('4735489: under maintenance','4735489: changed'))
 def test_guarded_v5_upgrade(self):
  h=(R/'dashboard/card.html').read_text().replace('Live read-only Testnet and Mainnet observer telemetry. Select a network below; refreshes every 30 seconds while visible.','Live read-only mainnet observer status.').replace('HVM_NETWORK_CARD_V5_2_EN','HVM_NETWORK_CARD_V5_EN')
  old=(R/'dashboard'/ 'v4-card.html').read_text()
  # Synthetic exact V5 guard shape; installer must accept known markers and reject drift.
  h='<div class="view" id="view-hvm"><div>Live read-only mainnet observer status</div><div>4735489 (0x484201)</div><button id="hvm-refresh"></button><div>Validator federation</div></div>'
  j="// HVM_NETWORK_CARD_V5_EN\nfetch('/api/hashburst/hvm/mainnet/network'); if(d.chain_id!==4735489) throw Error();"
  s='PREFIX\n'+h+'\n</main>\nKEEP\n'+j+"\nif (location.hash && document.getElementById('nav-' + location.hash.slice(1))) KEEP();\nSUFFIX"
  out=m.patch(s);self.assertIn('HVM_NETWORK_CARD_V5_2_EN',out);self.assertNotIn('HVM_NETWORK_CARD_V5_EN',out)
 def test_unknown_hvm_refused(self):
  with self.assertRaises(RuntimeError):m.patch(self.source()+'<button id="nav-hvm"></button>')
 def test_routes_are_dual_get_only_and_no_admin(self):
  r=(R/'dashboard/routes.conf').read_text();self.assertIn('/api/hashburst/hvm/mainnet/network',r);self.assertIn('127.0.0.1:18019/health',r);self.assertIn('/api/hashburst/hvm/testnet/network',r);self.assertIn('127.0.0.1:18009/health',r);self.assertGreaterEqual(r.count('limit_except GET'),2);self.assertNotIn('admin_peers',r);self.assertNotIn('77.90.188.',r)
 def test_card_cannot_regress_to_mainnet_only(self):
  h=(R/'dashboard/card.html').read_text();j=(R/'dashboard/card.js').read_text()
  for x in ('id="hvm-connect-mainnet"','id="hvm-connect-testnet"','P2P peers','Observer connected peers','Validator federation'):self.assertIn(x,h)
  for x in ("mainnet:{chain:4735489","testnet:{chain:4735490","/api/hashburst/hvm/mainnet/network","/api/hashburst/hvm/testnet/network","0x484201","0x484202"):self.assertIn(x,j)
 def test_ecosystem_uses_network_telemetry(self):
  out=m.patch(self.ecosystem(self.source()));self.assertIn("'/api/hashburst/hvm/mainnet/network'",out);self.assertIn("'/api/hashburst/hvm/testnet/network'",out);self.assertNotIn("'/api/hashburst/hvm/mainnet/health'",out);self.assertNotIn("'/api/hashburst/hvm/testnet/health'",out)
if __name__=='__main__':unittest.main()
