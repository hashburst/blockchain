import importlib.util,unittest
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('installer',R/'install-dashboard.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class PatchTests(unittest.TestCase):
 def source(self):return '<nav>\n    <button data-view="storage" id="nav-storage">Sovereign Storage</button>\n</nav><main></main><script nonce="preserved">\nloadView(\'explorer\');\n</script>'
 def test_install_idempotence(self):
  s=m.patch(self.source());self.assertEqual(s,m.patch(s));self.assertIn('HVM_NETWORK_CARD_V5_EN',s);self.assertIn('4735489 (0x484201)',s);self.assertIn('nonce="preserved"',s)
 def test_exact_v4_upgrade(self):
  s=m.patch(self.source())
  for name in ('card.html','card.js'):s=s.replace((R/'dashboard'/name).read_text(),(R/'dashboard'/('v4-'+name)).read_text())
  self.assertIn('HVM_NETWORK_CARD_V5_EN',m.patch(s))
 def test_guarded_v3_upgrade(self):
  h='<div class="view" id="view-hvm">HashBurst HVM Testnet 4735490 (0x484202)<span id="hvm-peers"></span>4735489: under maintenance</div>'
  j="// HVM_NETWORK_CARD_V3\nfetch('/api/hashburst/hvm/testnet/network'); if (d.chain_id !== 4735490) throw Error();"
  s='PREFIX\n'+h+'\n</main>\nKEEP\n'+j+"\nif (location.hash && document.getElementById('nav-' + location.hash.slice(1))) KEEP();\nSUFFIX"
  out=m.patch(s);self.assertTrue(out.startswith('PREFIX\n'));self.assertTrue(out.endswith('\nSUFFIX'));self.assertIn('\nKEEP\n',out);self.assertIn('HVM_NETWORK_CARD_V5_EN',out);self.assertNotIn('HVM_NETWORK_CARD_V3',out)
  with self.assertRaises(RuntimeError):m.patch(s.replace('4735489: under maintenance','4735489: changed'))
 def test_unknown_hvm_refused(self):
  with self.assertRaises(RuntimeError):m.patch(self.source()+'<button id="nav-hvm"></button>')
 def test_routes_are_get_only_and_no_admin(self):
  r=(R/'dashboard/routes.conf').read_text();self.assertIn('/api/hashburst/hvm/mainnet/network',r);self.assertIn('127.0.0.1:18019/health',r);self.assertIn('limit_except GET',r);self.assertNotIn('admin_peers',r);self.assertNotIn('77.90.188.',r)
 def test_card_has_distinct_peer_and_federation_semantics(self):
  h=(R/'dashboard/card.html').read_text();self.assertIn('P2P peers',h);self.assertIn('Observer connected peers',h);self.assertIn('Validator federation',h);self.assertIn('4 / 4',h)
if __name__=='__main__':unittest.main()
