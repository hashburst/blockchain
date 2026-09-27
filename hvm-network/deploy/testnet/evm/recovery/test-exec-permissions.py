"""Regression: a dedicated user cannot execute below a root-only release directory."""
import errno
import itertools
import importlib.util
import json
import os
import pwd
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

R = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('exec_permissions', R / 'exec-permissions.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class ExecPermissions(unittest.TestCase):
    @unittest.skipUnless(os.geteuid() == 0, 'actual uid transition requires root on Linux')
    def test_real_dedicated_user_denied_then_allowed_without_secret_access(self):
        nobody = pwd.getpwnam('nobody')
        try:
            subprocess.run(['/bin/true'], check=True, user=nobody.pw_uid, group=nobody.pw_gid, extra_groups=[])
        except OSError as exc:
            if exc.errno in (errno.EPERM, errno.EINVAL):
                self.skipTest('sandbox disallows UID/GID transitions; real probe remains mandatory on VPS')
            raise
        with tempfile.TemporaryDirectory() as d:
            parent = Path(d); parent.chmod(0o755)
            release = parent / 'release'; release.mkdir(mode=0o700)
            binary = release / 'runtime'
            binary.write_text('#!/bin/sh\ntest "$1" = --help\n')
            binary.chmod(0o755)
            secret = release / 'candidate.json'; secret.write_text('private'); secret.chmod(0o600)
            original_sha = m.executable_sha(binary)
            with self.assertRaises(PermissionError):
                m.probe_exec(binary, 'nobody', '')
            release.chmod(0o755)
            m.probe_exec(binary, 'nobody', '')
            denied = subprocess.run(['/bin/cat', str(secret)], capture_output=True,
                                    user=nobody.pw_uid, group=nobody.pw_gid, extra_groups=[])
            self.assertNotEqual(denied.returncode, 0)
            self.assertEqual(m.executable_sha(binary), original_sha)
            self.assertEqual(stat.S_IMODE(secret.stat().st_mode), 0o600)

    def test_running_correct_binary_is_not_restarted(self):
        with patch.object(m, 'executable_sha', return_value='sha'):
            self.assertEqual(m.launch_decision({'MainPID':'23'}, Path('/bin/runtime'), 'sha', False), 'already-running')

    def test_running_other_binary_is_retained(self):
        with patch.object(m, 'executable_sha', return_value='different'):
            with self.assertRaisesRegex(RuntimeError, 'different binary'):
                m.launch_decision({'MainPID':'23'}, Path('/bin/runtime'), 'sha', False)

    def test_unrelated_failure_not_restarted(self):
        with self.assertRaisesRegex(RuntimeError, 'not the observed'):
            m.launch_decision({'MainPID':'0','ExecMainStatus':'1','ActiveState':'failed'}, Path('/bin/runtime'), 'sha', False)

    def test_exact_exec_failure_and_recorded_interruption(self):
        self.assertEqual(m.launch_decision({'MainPID':'0','ExecMainStatus':'203'}, Path('/bin/runtime'), 'sha', False), 'repair-exec')
        self.assertEqual(m.launch_decision({'MainPID':'0','ExecMainStatus':'0','ActiveState':'inactive'}, Path('/bin/runtime'), 'sha', True), 'resume-recorded-start')

    def test_preflight_and_already_running_have_no_mutations(self):
        for action,decision in [('exec-preflight','repair-exec'),('exec-repair','already-running')]:
            scope=(Path('/release'),Path('/release/runtime'),'test.service',{'User':'dedicated'},Path('/marker'),decision,0o700)
            with patch.object(m,'exec_scope',return_value=scope),patch.object(m.subprocess,'run') as run,patch.object(m.os,'chmod') as chmod:
                m.exec_action({'action':action,'node':{'node_id':'v1'},'sha256':'sha'})
                run.assert_not_called();chmod.assert_not_called()

    def test_guarded_repair_changes_only_release_mode_and_records_start(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);root.chmod(0o700)
            secret=root/'candidate.json';secret.write_text('unchanged');secret.chmod(0o600)
            marker=root/'exec-permissions-proof.json'
            scope=(root,root/'runtime','test.service',{'User':'dedicated','Group':'dedicated'},marker,'repair-exec',0o700)
            properties=[{'MainPID':'0','ExecMainStatus':'203'},
                        {'MainPID':'0','ActiveState':'inactive'}]+[{'MainPID':'99','ActiveState':'active','SubState':'running'}]*10
            clock=itertools.count()
            with patch.object(m,'exec_scope',return_value=scope),patch.object(m,'service_properties',side_effect=properties),patch.object(m,'trusted_path'),patch.object(m,'probe_exec') as probe,patch.object(m,'base_main',create=True) as preserve,patch.object(m.subprocess,'run') as run,patch.object(m,'executable_sha',return_value='sha'),patch.object(m.time,'sleep'),patch.object(m.time,'monotonic',side_effect=lambda:next(clock)):
                out=m.exec_action({'action':'exec-repair','node':{'node_id':'v1'},'sha256':'sha'})
            self.assertTrue(out['process_started'])
            self.assertEqual(stat.S_IMODE(root.stat().st_mode),0o755)
            self.assertEqual(stat.S_IMODE(secret.stat().st_mode),0o600)
            self.assertEqual(secret.read_text(),'unchanged')
            self.assertEqual(stat.S_IMODE(marker.stat().st_mode),0o600)
            self.assertEqual(json.loads(marker.read_text())['directory_mode'],'0o700')
            self.assertEqual([c.args[0][1] for c in run.call_args_list],['stop','reset-failed','start'])
            probe.assert_called_once();preserve.assert_called_once()

    def test_private_or_symlink_release_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'link';p.symlink_to('/bin/true')
            with self.assertRaisesRegex(RuntimeError,'untrusted'):
                m.trusted_path(p)

    def test_no_config_or_data_execution_in_probe(self):
        account=pwd.getpwuid(os.geteuid())
        with patch.object(m.subprocess,'run',return_value=subprocess.CompletedProcess([],0,'','')) as run:
            m.probe_exec('/opt/runtime', account.pw_name, '')
            self.assertEqual(run.call_args.args[0],['/opt/runtime','--help'])

if __name__=='__main__':unittest.main()
