import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('rollout', Path(__file__).with_name('rollout-archive.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class RolloutTests(unittest.TestCase):
    def test_sequence_and_inflight_are_enforced(self):
        s = {'artifact': 'a', 'completed': [], 'in_flight': None}
        with self.assertRaises(RuntimeError):
            m.reserve(s, m.HOSTS[1], 'a')
        m.reserve(s, m.HOSTS[0], 'a')
        with self.assertRaises(RuntimeError):
            m.reserve(s, m.HOSTS[1], 'a')
        with self.assertRaises(RuntimeError):
            m.reserve(s, m.HOSTS[0], 'other-installer')
        self.assertEqual(s['completed'], [])

    def test_interrupted_worker_resumes_with_originals_preserved(self):
        # Inject a crash after durable mask publication, then after archive unit
        # publication. Exercise actual filesystem changes; systemd is mocked.
        for failpoint in ('masked', 'configured'):
            with self.subTest(failpoint=failpoint), tempfile.TemporaryDirectory() as td:
                root = Path(td).resolve()
                stages = root / 'stages'
                stage = stages / ('a' * 64)
                stage.mkdir(parents=True)
                unit = root / m.UNIT
                original = b'[Service]\nExecStart=/usr/local/bin/hashburst-node\n'
                unit.write_bytes(original)
                drops = root / (m.UNIT + '.d')
                drops.mkdir()
                (drops / 'override.conf').write_text('[Service]\nEnvironment=LEGACY=1\n')
                ledger = root / 'ledger'
                ledger.mkdir()
                for name in m.PINS:
                    (stage / name).write_bytes(b'terminal')
                    (ledger / name).write_bytes(b'original-chain')
                (stage / 'hvm-legacy-archive').write_bytes(b'binary')
                sha = m.digest(stage / 'hvm-legacy-archive')
                install = root / 'install'
                baseline = {'hvm-testnet.service': {'MainPID': '777', 'ActiveState': 'active'}}
                failed = False
                calls = []
                def command(*args, **kwargs):
                    nonlocal failed
                    calls.append(args)
                    if args == ('systemctl', 'daemon-reload') and not failed:
                        atpoint = unit.is_symlink() if failpoint == 'masked' else not unit.is_symlink() and b'DynamicUser=yes' in unit.read_bytes()
                        if atpoint:
                            failed = True
                            raise RuntimeError('simulated interruption')
                    return ''
                props = {'FragmentPath': str(unit), 'DropInPaths': '', 'ConsistsOf': '', 'BoundBy': '', 'PropagatesStopTo': ''}
                with patch.multiple(m, UNIT_FILE=unit, STAGE_ROOT=stages, INSTALL_ROOT=install,
                                    LEDGER_ROOT=ledger, LOCK_PATH=root / 'lock'), \
                     patch.object(m.os, 'geteuid', return_value=0), \
                     patch.object(m, 'properties', return_value=props), \
                     patch.object(m, 'protected', return_value=baseline), \
                     patch.object(m, 'check_pair'), patch.object(m, 'no_old_writer'), \
                     patch.object(m, 'api_check'), patch.object(m, 'verify') as verify, \
                     patch.object(m, 'run', side_effect=command):
                    with self.assertRaisesRegex(RuntimeError, 'simulated interruption'):
                        m.worker(stage, sha)
                    self.assertTrue(failed)
                    m.worker(stage, sha)
                    self.assertEqual(json.loads((stage / 'state.json').read_text())['phase'], 'verified')
                    self.assertEqual((stage / 'backup/unit.original').read_bytes(), original)
                    for name in m.PINS:
                        self.assertEqual((ledger / name).read_bytes(), b'original-chain')
                        self.assertEqual((stage / 'backup' / name).read_bytes(), b'original-chain')
                    self.assertFalse(drops.exists())
                    self.assertTrue((stage / 'backup/dropins.disabled/override.conf').exists())
                    self.assertEqual(unit.read_bytes(), m.unit_text(install / sha))
                    verify.assert_called_once()
                    self.assertNotIn(('systemctl', 'restart', m.UNIT), calls)
                    verify.reset_mock()
                    # Check-only performs verification and cannot install/start anything.
                    calls.clear()
                    m.worker(stage, sha, True)
                    verify.assert_called_once()
                    self.assertEqual(calls, [])

    def test_existing_digest_change_refused_before_service_stop(self):
        with tempfile.TemporaryDirectory() as td:
            stage = Path(td).resolve() / 'stage'
            stage.mkdir()
            (stage / 'hvm-legacy-archive').write_bytes(b'wrong')
            with patch.multiple(m, STAGE_ROOT=stage.parent, LOCK_PATH=stage / 'lock'), \
                 patch.object(m.os, 'geteuid', return_value=0), patch.object(m, 'run') as run:
                with self.assertRaisesRegex(RuntimeError, 'candidate differs'):
                    m.worker(stage, 'a' * 64)
                run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
