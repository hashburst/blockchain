import copy
import json
from pathlib import Path
import unittest
from migration_model import apply_import, state_commitment, RECIPIENT, FOUNDER, UNITS

class ImportTests(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads(Path(__file__).with_name('legacy-migration.candidate.json').read_text())['import']
        self.state = {'chain_id': 4735489, 'balances_units': {RECIPIENT: str(FOUNDER)}, 'consumed_sources': []}
    def test_conservation_and_restart_replay_rejection(self):
        result = apply_import(self.state, self.manifest)
        self.assertEqual(int(result['balances_units'][RECIPIENT]), FOUNDER + UNITS)
        restored = json.loads(json.dumps(result))
        root = state_commitment(restored)
        with self.assertRaisesRegex(ValueError, 'already imported'):
            apply_import(restored, self.manifest)
        self.assertEqual(state_commitment(restored), root)
        self.assertEqual(self.state['consumed_sources'], [])
    def test_report_change_does_not_allow_double_import(self):
        result = apply_import(self.state, self.manifest)
        altered = {**self.manifest, 'report_sha256': 'different observation'}
        with self.assertRaisesRegex(ValueError, 'already imported'):
            apply_import(result, altered)
    def test_rejects_altered_economics_without_mutation(self):
        for key, value in [('amount_units', str(UNITS + 1)), ('recipient', '0x' + '0'*40),
                           ('target_chain_id', 4735490), ('source_hash', '0'*64),
                           ('additional_migration_issuance_units', '1'), ('source_nullifier', '0'*64)]:
            with self.subTest(key=key):
                before = copy.deepcopy(self.state)
                with self.assertRaises(ValueError):
                    apply_import(self.state, {**self.manifest, key: value})
                self.assertEqual(self.state, before)
    def test_wrong_chain_and_overflow(self):
        for state in [{**self.state, 'chain_id': 1337},
                      {**self.state, 'balances_units': {RECIPIENT: str(2**63-1)}}]:
            with self.assertRaises(ValueError):
                apply_import(state, self.manifest)

if __name__ == '__main__':
    unittest.main()
