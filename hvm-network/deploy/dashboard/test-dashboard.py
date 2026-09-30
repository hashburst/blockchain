import importlib.util
import unittest
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('installer',R/'install-dashboard.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class PatchTests(unittest.TestCase):
 def source(self):
  return '<nav>\n    <button data-view="storage" id="nav-storage">Sovereign Storage</button>\n</nav><main></main><script nonce="preserved">\nloadView(\'explorer\');\n</script>'
 def test_install_and_idempotence(self):
  s=m.patch(self.source());self.assertEqual(s,m.patch(s));self.assertIn('nonce="preserved"',s);self.assertEqual(s.count('id="nav-hvm"'),1);self.assertNotIn('data-hvm-it',s);self.assertNotIn('hvm-it',s);self.assertIn('<th>Legacy</th><td>1337</td>',s)
 def test_unknown_page_refused(self):
  with self.assertRaises(RuntimeError):m.patch('<main></main>')
 def test_old_panel_refused(self):
  with self.assertRaises(RuntimeError):m.patch(self.source()+'<button id="nav-hvm"></button>')
 def test_duplicate_anchor_refused(self):
  with self.assertRaises(RuntimeError):m.patch(self.source()+'</main>')
 def test_known_previous_version_upgrade(self):
  text=m.patch(self.source())
  for name in ('card.html','card.js'):
   text=text.replace((R/'dashboard'/name).read_text(),(R/'dashboard'/('previous-'+name)).read_text())
  updated=m.patch(text)
  self.assertIn('HVM_NETWORK_CARD_V4_EN',updated)
  self.assertNotIn('HVM_NETWORK_CARD_V2',updated)
  self.assertIn('nonce="preserved"',updated)
  self.assertEqual(m.patch(updated),updated)
 def test_v3_upgrade(self):
  text=m.patch(self.source())
  for name in ('card.html','card.js'):
   text=text.replace((R/'dashboard'/name).read_text(),(R/'dashboard'/('v3-'+name)).read_text())
  updated=m.patch(text)
  self.assertIn('HVM_NETWORK_CARD_V4_EN',updated)
  self.assertNotIn('hvm-it',updated)
  self.assertEqual(updated,m.patch(updated))
if __name__=='__main__':unittest.main()
