"""Repair only the root-owned release traversal defect after completed migration."""
import grp
import hashlib
import json
import os
import pwd
import stat
import subprocess
import time
from pathlib import Path


def executable_sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for chunk in iter(lambda: f.read(1048576), b''):
            h.update(chunk)
    return h.hexdigest()


def trusted_path(path, directory=False):
    path = Path(path)
    for parent in (path, *path.parents):
        st = parent.lstat()
        if stat.S_ISLNK(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o022:
            raise RuntimeError('untrusted release path: ' + str(parent))
    st = path.stat()
    if directory and not stat.S_ISDIR(st.st_mode):
        raise RuntimeError('release is not a directory')
    if not directory and not stat.S_ISREG(st.st_mode):
        raise RuntimeError('release file is not regular')
    return stat.S_IMODE(st.st_mode)


def service_properties(service):
    props = ('User', 'Group', 'DynamicUser', 'MainPID', 'ActiveState', 'SubState',
             'ExecMainStatus', 'NRestarts', 'ExecStart', 'RootDirectory', 'RootImage',
             'NoExecPaths', 'ExecPaths', 'LoadState', 'FragmentPath')
    args = ['systemctl', 'show', service]
    for key in props:
        args.extend(['-p', key])
    out = subprocess.check_output(args, text=True, timeout=20)
    return dict(line.split('=', 1) for line in out.splitlines() if '=' in line)


class PendingExecution(RuntimeError):
    """A sampled MainPID has not yet been confirmed as the target executable."""


def launch_decision(properties, binary, sha, marker_exists):
    pid = int(properties.get('MainPID', '0'))
    if pid:
        actual = executable_sha(Path('/proc') / str(pid) / 'exe')
        if actual != sha:
            raise PendingExecution('existing process has a different binary; retained; pid=' + str(pid) + ' sha256=' + actual)
        return 'already-running'
    if properties.get('ExecMainStatus') == '203':
        return 'repair-exec'
    if marker_exists and properties.get('ActiveState') in ('inactive', 'failed'):
        return 'resume-recorded-start'
    if properties.get('ActiveState') in ('activating', 'active', 'deactivating'):
        raise PendingExecution('service transition; no confirmed executable yet')
    raise RuntimeError('not the observed 203/EXEC failure; service retained')


def settled_launch_decision(service, binary, sha, marker_exists, timeout=15):
    # Type=simple may publish MainPID before exec; /proc may still identify
    # systemd's executor or disappear between samples. Never stop that PID.
    deadline = time.monotonic() + timeout
    last = ''
    while True:
        properties = service_properties(service)
        try:
            decision = launch_decision(properties, binary, sha, marker_exists)
            return properties, decision
        except (PendingExecution, FileNotFoundError, ProcessLookupError, PermissionError) as exc:
            last = str(exc)
        if time.monotonic() >= deadline:
            raise RuntimeError('EXEC_IDENTITY_UNCONFIRMED; no service changed; ' + last +
                               '; last_service_state=' + json.dumps(properties))
        time.sleep(0.5)


def reset_failed_if_needed(service):
    state = service_properties(service)
    if state.get('ActiveState') != 'failed':
        return False
    result = subprocess.run(['systemctl', 'reset-failed', service],
                            capture_output=True, text=True, timeout=15)
    if result.returncode:
        # A stopped unit can be unloaded between the query and reset-failed.
        # Only a fresh, loaded inactive unit is an acceptable disappearance.
        after = service_properties(service)
        if after.get('LoadState') != 'loaded' or after.get('ActiveState') != 'inactive' or int(after.get('MainPID', '0')):
            raise RuntimeError('reset-failed did not clear failure: ' + result.stderr.strip() +
                               '; state=' + json.dumps(after))
    return True


def probe_exec(binary, user, group):
    account = pwd.getpwnam(user)
    gid = grp.getgrnam(group).gr_gid if group else account.pw_gid
    groups = os.getgrouplist(user, gid)
    # --help exits during flag parsing, before loading config, state or keys.
    r = subprocess.run([str(binary), '--help'], cwd='/', capture_output=True,
                       text=True, timeout=15, user=account.pw_uid,
                       group=gid, extra_groups=groups)
    if r.returncode:
        raise RuntimeError('service-user executable probe failed: ' + r.stderr[-1500:])


def exec_scope(p):
    n = p['node']
    prefix = 'hashburst-hvm-testnet' + ('-ingress' if n['role'] == 'observer' else '')
    service = prefix + '.service'
    cfg = Path('/etc') / prefix / 'node.json'
    c = json.loads(cfg.read_text())
    if (c['node_id'], c['role'], c['protocol']['chain_id']) != (n['node_id'], n['role'], 4735490):
        raise RuntimeError('identity/network mismatch')
    if c['protocol'].get('evm') != p['evm']:
        raise RuntimeError('migration not installed; no automatic migration')
    if n['role'] == 'observer' and c.get('consensus_key_file'):
        raise RuntimeError('observer signing key configured')
    root = Path('/opt') / prefix / ('evm-repair-' + p['sha256'][:12])
    mode = trusted_path(root, directory=True)
    if mode not in (0o700, 0o755):
        raise RuntimeError('unexpected release directory permissions: ' + oct(mode))
    binary = root / 'hashburst-testnet'
    if trusted_path(binary) != 0o755 or executable_sha(binary) != p['sha256']:
        raise RuntimeError('binary permissions/checksum mismatch')
    for name in ('candidate.json', 'job.json', 'offline.py'):
        if trusted_path(root / name) != 0o600:
            raise RuntimeError('private release file must remain 0600: ' + name)
    trusted_path(root / 'result.json')
    r = json.loads((root / 'result.json').read_text())
    if r.get('ok') is not True or r.get('node_id') != n['node_id'] or r.get('binary_sha256') != p['sha256'] or r.get('evm') != p['evm']:
        raise RuntimeError('completed offline evidence mismatch')
    expected = '[Service]\nExecStart=\nExecStart=' + str(binary) + ' --config ' + str(cfg) + '\n'
    if (Path('/etc/systemd/system') / (service + '.d') / '50-evm-release.conf').read_text() != expected:
        raise RuntimeError('release override differs')
    props = service_properties(service)
    if str(binary) not in props.get('ExecStart', '') or str(cfg) not in props.get('ExecStart', ''):
        raise RuntimeError('effective ExecStart differs')
    if not props.get('User') or props['User'] == 'root' or props.get('DynamicUser') == 'yes':
        raise RuntimeError('expected a dedicated static service user')
    if any(props.get(key) for key in ('RootDirectory', 'RootImage', 'NoExecPaths', 'ExecPaths')):
        raise RuntimeError('additional execution restrictions require review; hardening retained')
    marker = root / 'exec-permissions-proof.json'
    if marker.exists():
        trusted_path(marker)
        saved = json.loads(marker.read_text())
        if saved.get('binary_sha256') != p['sha256'] or saved.get('node_id') != n['node_id']:
            raise RuntimeError('permission repair evidence differs')
    props, decision = settled_launch_decision(service, binary, p['sha256'], marker.exists())
    base_main(dict(p, action='preservation'))
    return root, binary, service, props, marker, decision, mode


def exec_action(p):
    root, binary, service, props, marker, decision, mode = exec_scope(p)
    result = {'node_id': p['node']['node_id'], 'decision': decision,
              'directory_mode': oct(mode), 'service_user': props['User'],
              'offline_gate_verified': True, 'journal_prefixes_verified': True}
    if p['action'] == 'exec-preflight':
        return result
    if decision == 'already-running':
        return dict(result, no_restart=True)
    # Record first, so an interrupted stop/start can be resumed without a migration.
    if not marker.exists():
        fd = os.open(marker, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, 'w') as f:
            json.dump(dict(result, binary_sha256=p['sha256'], timestamp=time.time()), f)
            f.flush()
            os.fsync(f.fileno())
    # Recheck immediately before stopping the known EXEC crash loop.
    current, current_decision = settled_launch_decision(service, binary, p['sha256'], True)
    if current_decision == 'already-running':
        return dict(result, no_restart=True)
    subprocess.run(['systemctl', 'stop', service], check=True, timeout=30)
    stopped = service_properties(service)
    if int(stopped.get('MainPID', '0')) or stopped.get('ActiveState') not in ('inactive', 'failed'):
        raise RuntimeError('service not stopped; permissions retained')
    trusted_path(root, directory=True)
    os.chmod(root, 0o755)  # Only this release directory; no recursive chmod.
    probe_exec(binary, props['User'], props.get('Group', ''))
    base_main(dict(p, action='preservation'))
    reset_failed_if_needed(service)
    subprocess.run(['systemctl', 'start', '--no-block', service], check=True, timeout=15)
    # Prove a real process, without waiting here for long canonical replay.
    deadline = time.monotonic() + 40
    stable_pid = None
    stable_since = None
    while time.monotonic() < deadline:
        now = service_properties(service)
        pid = int(now.get('MainPID', '0'))
        if pid:
            try:
                matches = executable_sha(Path('/proc') / str(pid) / 'exe') == p['sha256']
            except FileNotFoundError:
                matches = False
            if matches:
                if pid != stable_pid:
                    stable_pid, stable_since = pid, time.monotonic()
                elif time.monotonic() - stable_since >= 3:
                    return dict(result, permissions_repaired=True, process_started=True,
                                pid=pid, readiness='replay/finality verification follows')
        if now.get('ActiveState') in ('failed', 'inactive') or (now.get('SubState') == 'auto-restart' and now.get('ExecMainStatus') != '0'):
            raise RuntimeError('runtime startup failed after permission repair: ' + json.dumps(now))
        time.sleep(1)
    raise RuntimeError('process start not confirmed; services retained; inspect logs')
