import importlib.util
from pathlib import Path
import unittest

s = importlib.util.spec_from_file_location('accept', Path(__file__).with_name('accept-ha-archive.py'))
m = importlib.util.module_from_spec(s)
s.loader.exec_module(m)

class TransitionTests(unittest.TestCase):
    def test_only_reviewed_transition_allowed(self):
        before = {m.MASTER: {'ActiveState': 'active', 'MainPID': '79801'}}
        after = {m.MASTER: {'ActiveState': 'active', 'MainPID': '3206610'}}
        self.assertTrue(m.classify('77.90.188.153', before, after))
        self.assertFalse(m.classify('77.90.188.153', before, before))
        for host, value in [('64.31.4.9', after), ('77.90.188.153', {}),
                            ('77.90.188.153', {m.MASTER: {'ActiveState': 'active', 'MainPID': '999'}}),
                            ('77.90.188.153', {**after, 'hashburst-tep.service': {'ActiveState': 'failed'}})]:
            with self.subTest(host=host, value=value), self.assertRaises(RuntimeError):
                m.classify(host, before, value)

if __name__ == '__main__':
    unittest.main()
