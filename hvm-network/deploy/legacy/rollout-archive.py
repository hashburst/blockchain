#!/usr/bin/env python3
"""One-host legacy archive rollout. Never restores a writable legacy runtime.
Run from the Mac; SSH/SCP prompt normally. No secrets are read by this program.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time
import urllib.request
import urllib.error

HOSTS = ('64.31.4.9', '77.90.188.157', '77.90.188.155', '77.90.188.154', '77.90.188.153')
UNIT = 'hashburst-node.service'
UNIT_FILE = Path('/etc/systemd/system') / UNIT
STAGE_ROOT = Path('/root/hashburst-archive-rollout')
INSTALL_ROOT = Path('/opt/hashburst-legacy-archive')
LEDGER_ROOT = Path('/var/lib/hashburst')
LOCK_PATH = Path('/run/hashburst-legacy-archive.lock')
PINS = {'blockchain.dat': 'd6eda91edad1b4858a1b3ddd97347f468aab998168ec818eefa67d5becabc06b',
        'blockchain.idx': 'e2ff5d9437fe2f789197e3fe5a8863ed25cc5f5a5c3ee17c5218ee511508d68f'}
TERMINAL = '0000a8bef0916f373a97fa8c311b095258f00cf7f2a24a3ff2f875156ca27ae1'
ROOT = '33f6f441083e985f75502269b8c7e9bb4e621c43286c8232ba17b8f36e57de81'
RECIPIENT = '0xd1da8d04d767685e53440dbc56803af350a65333'
SOURCE = '/root/hashburst-terminal-74338c75'
SSH_OPTIONS = ['-o', 'ControlMaster=no', '-o', 'ControlPath=none', '-o', 'ConnectTimeout=15',
               '-o', 'ServerAliveInterval=30', '-o', 'ServerAliveCountMax=3',
               '-o', 'PubkeyAuthentication=no', '-o', 'PreferredAuthentications=keyboard-interactive,password']


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def run(*args, capture=False, timeout=120):
    return subprocess.run(args, check=True, text=True, stdout=subprocess.PIPE if capture else None,
                          timeout=timeout).stdout


def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def syncdir(path):
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def atomic(path, data, mode=0o600):
    path = Path(path)
    tmp = path.with_name(path.name + '.new')
    require(not tmp.is_symlink(), 'temporary path is a symlink')
    with open(tmp, 'wb') as f:
        os.fchmod(f.fileno(), mode)
        f.write(data)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)
    syncdir(path.parent)


def save(path, data):
    atomic(path, (json.dumps(data, indent=2, sort_keys=True) + '\n').encode())


def load(path, default=None):
    return json.loads(Path(path).read_text()) if Path(path).exists() else default


def properties(unit):
    return dict(line.split('=', 1) for line in run('systemctl', 'show', unit,
        '-p', 'MainPID', '-p', 'ActiveState', '-p', 'FragmentPath', '-p', 'DropInPaths',
        '-p', 'ConsistsOf', '-p', 'PropagatesStopTo', '-p', 'BoundBy', capture=True).splitlines() if '=' in line)


def protected():
    lines = run('systemctl', 'list-units', '--type=service', '--all', '--no-legend', '--plain', capture=True)
    names = [line.split()[0] for line in lines.splitlines() if line.split()]
    return {name: {k: properties(name)[k] for k in ('MainPID', 'ActiveState')}
            for name in names if name != UNIT and any(s in name.lower() for s in ('hashburst', 'hvm', 'ipfs', 'kubo', 'tep', 'hb-files'))}


def unit_text(directory):
    command = [str(directory / 'hvm-legacy-archive'), '--directory', str(directory),
               '--dat-sha256', PINS['blockchain.dat'], '--idx-sha256', PINS['blockchain.idx'],
               '--terminal-hash', TERMINAL, '--listen', '127.0.0.1:8009']
    return ('[Unit]\nDescription=HashBurst 1337 immutable terminal archive\nAfter=network.target\n'
            'StartLimitIntervalSec=60\nStartLimitBurst=3\n[Service]\nType=simple\nDynamicUser=yes\nExecStart=' +
            ' '.join(command) + '\nRestart=on-failure\nRestartSec=5\nNoNewPrivileges=yes\nProtectSystem=strict\n'
            'ProtectHome=yes\nPrivateTmp=yes\nPrivateDevices=yes\nProtectKernelTunables=yes\nProtectKernelModules=yes\n'
            'ProtectControlGroups=yes\nRestrictSUIDSGID=yes\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n'
            'CapabilityBoundingSet=\nUMask=0077\nCPUWeight=10\nMemoryMax=512M\nTasksMax=128\nLimitNOFILE=1024\n'
            'StandardOutput=journal\nStandardError=journal\n[Install]\nWantedBy=multi-user.target\n').encode()


def check_pair(directory):
    for name, pin in PINS.items():
        path = directory / name
        require(path.is_file() and not path.is_symlink() and digest(path) == pin, 'terminal file mismatch: ' + name)


def api_check():
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    def get(route):
        with opener.open('http://127.0.0.1:8009/api/' + route, timeout=10) as f:
            return json.load(f)
    h = get('health')
    expected = {'chainId': 1337, 'blockHeight': 11, 'terminal_height': 10, 'terminal_hash': TERMINAL,
                'state_root': ROOT, 'mode': 'legacy-terminal-archive', 'spendable': False,
                'mining_enabled': False, 'p2p_enabled': False}
    require(all(k in h and h[k] == v for k, v in expected.items()), 'archive health mismatch')
    balances = get('balances')
    require(balances.get('balance_units') == {RECIPIENT: 45000000000} and
            balances.get('unit_decimals') == 8 and balances.get('spendable') is False, 'balance mismatch')
    blocks = get('blocks')
    require(len(blocks) == 11 and blocks[-1]['hash'] == TERMINAL, 'archive blocks mismatch')
    for route in ('transactions', 'mine', 'register', 'import'):
        try:
            opener.open(urllib.request.Request('http://127.0.0.1:8009/api/' + route, data=b'{}', method='POST'), timeout=10).close()
        except urllib.error.HTTPError as e:
            require(e.code == 405, 'write rejection differs: ' + route)
        else:
            raise RuntimeError('write API accepted: ' + route)
    return h


def no_old_writer():
    for entry in Path('/proc').iterdir():
        if entry.name.isdigit():
            try:
                target = os.readlink(entry / 'exe')
            except (FileNotFoundError, ProcessLookupError):
                continue
            require(Path(target.removesuffix(' (deleted)')).name != 'hashburst-node', 'unmanaged legacy writer still running')


def verify(stage, binary_sha, directory):
    require(UNIT_FILE.is_file() and not UNIT_FILE.is_symlink() and UNIT_FILE.read_bytes() == unit_text(directory), 'unit changed')
    props = properties(UNIT)
    require(props['ActiveState'] == 'active' and not props['DropInPaths'] and props['FragmentPath'] == str(UNIT_FILE), 'unexpected effective service')
    pid = int(props['MainPID'])
    require(pid > 0 and digest('/proc/%s/exe' % pid) == binary_sha, 'running binary differs')
    check_pair(directory)
    no_old_writer()
    h = api_check()
    baseline = load(stage / 'protected.json')
    require(baseline is not None and protected() == baseline, 'protected service state/PID changed; inspect before continuing')
    report = {'verified_at_unix': int(time.time()), 'service': UNIT, 'binary_sha256': binary_sha,
              'health': h, 'files': PINS, 'managed_host_archive_verified': True,
              'scope': 'this managed service on this host only', 'fleet_freeze_verified': False,
              'mainnet_import_executed': False, 'protected_services_unchanged': True}
    save(stage / 'ACCEPTANCE.json', report)
    return report


def worker(stage, binary_sha, check_only=False):
    require(os.geteuid() == 0, 'root required')
    require(re.fullmatch('[0-9a-f]{64}', binary_sha) is not None, 'invalid digest')
    require(stage.resolve() == stage and stage.parent == STAGE_ROOT, 'unexpected stage')
    with open(LOCK_PATH, 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        directory = INSTALL_ROOT / binary_sha
        if check_only:
            verify(stage, binary_sha, directory)
            return
        require(digest(stage / 'hvm-legacy-archive') == binary_sha, 'candidate differs')
        check_pair(stage)
        directory.mkdir(parents=True, exist_ok=True, mode=0o755)
        os.chmod(directory.parent, 0o755)
        os.chmod(directory, 0o755)
        for name in ('hvm-legacy-archive', *PINS):
            dest = directory / name
            if dest.exists():
                require(not dest.is_symlink() and digest(dest) == digest(stage / name), 'installed artifact differs')
            else:
                atomic(dest, (stage / name).read_bytes(), 0o755 if name == 'hvm-legacy-archive' else 0o444)
        run(str(directory / 'hvm-legacy-archive'), '--directory', str(directory), '--dat-sha256', PINS['blockchain.dat'],
            '--idx-sha256', PINS['blockchain.idx'], '--terminal-hash', TERMINAL, '--check')
        backup = stage / 'backup'
        backup.mkdir(mode=0o700, exist_ok=True)
        statepath = stage / 'state.json'
        def phase(value):
            save(statepath, {'phase': value, 'binary_sha256': binary_sha})
        configured = not UNIT_FILE.is_symlink() and UNIT_FILE.exists() and UNIT_FILE.read_bytes() == unit_text(directory)
        if not configured:
            masked = UNIT_FILE.is_symlink() and os.readlink(UNIT_FILE) == '/dev/null'
            if not masked:
                require(UNIT_FILE.is_file() and not UNIT_FILE.is_symlink(), 'regular legacy unit required')
                p = properties(UNIT)
                require(p['FragmentPath'] == str(UNIT_FILE), 'unexpected legacy fragment')
                require(not any(p[k] for k in ('ConsistsOf', 'PropagatesStopTo', 'BoundBy')), 'stop dependencies require review')
                if not (stage / 'protected.json').exists():
                    save(stage / 'protected.json', protected())
                if not (backup / 'unit.original').exists():
                    atomic(backup / 'unit.original', UNIT_FILE.read_bytes())
                phase('stopping_legacy')
                run('systemctl', 'stop', UNIT, timeout=90)
                run('systemctl', 'disable', UNIT)
                tmp = UNIT_FILE.with_name(UNIT + '.mask-new')
                if tmp.is_symlink():
                    tmp.unlink()
                os.symlink('/dev/null', tmp)
                os.replace(tmp, UNIT_FILE)
                syncdir(UNIT_FILE.parent)
                run('systemctl', 'daemon-reload')
            require((backup / 'unit.original').is_file(), 'original unit backup missing')
            phase('legacy_masked')
            no_old_writer()
            for name in PINS:
                old = LEDGER_ROOT / name
                if old.exists() and not (backup / name).exists():
                    require(old.is_file() and not old.is_symlink(), 'legacy file is not regular')
                    atomic(backup / name, old.read_bytes())
            drops = UNIT_FILE.with_name(UNIT + '.d')
            if drops.exists():
                require(not drops.is_symlink() and not (backup / 'dropins.disabled').exists(), 'ambiguous overrides')
                os.rename(drops, backup / 'dropins.disabled')
                syncdir(drops.parent)
                syncdir(backup)
            atomic(UNIT_FILE, unit_text(directory), 0o644)
            run('systemctl', 'daemon-reload')
            phase('archive_configured')
        run('systemctl', 'daemon-reload')
        require(not properties(UNIT)['DropInPaths'], 'unexpected drop-in overrides')
        run('systemctl', 'enable', UNIT)
        run('systemctl', 'reset-failed', UNIT)
        run('systemctl', 'start', UNIT)
        phase('archive_start_requested')
        deadline = time.monotonic() + 120
        while True:
            try:
                api_check()
                break
            except (OSError, urllib.error.URLError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(3)
        verify(stage, binary_sha, directory)
        phase('verified')


def reserve(state, host, artifact):
    require(state['artifact'] == artifact, 'different artifact: finish or inspect the previous rollout')
    require(state.get('in_flight') in (None, host), 'another host is in flight')
    if host not in state['completed']:
        next_host = next((h for h in HOSTS if h not in state['completed']), None)
        require(host == next_host, 'next host must be ' + str(next_host))
    state['in_flight'] = host
    return state


def local(args):
    root = Path(args.deployer_root).resolve(strict=True)
    with open(root / '.legacy-archive-rollout.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        work = root / 'legacy-archive-rollout'
        work.mkdir(mode=0o700, exist_ok=True)
        script = Path(__file__).resolve()
        # Reuse the exact binary already tested on the source, rather than silently rebuilding it.
        binary = work / 'hvm-legacy-archive'
        def ssh(command):
            return run('ssh', *SSH_OPTIONS, 'root@' + args.host, command, capture=True, timeout=60)
        def scp(source, destination):
            run('scp', *SSH_OPTIONS, source, destination)
        for name, remote in [('hvm-legacy-archive', SOURCE + '/hvm-legacy-archive'),
                             *[(n, SOURCE + '/generation/' + n) for n in PINS]]:
            if not (work / name).exists():
                temp = work / (name + '.download')
                scp('root@64.31.4.9:' + remote, str(temp))
                os.replace(temp, work / name)
        check_pair(work)
        binary_sha = digest(binary)
        artifact = hashlib.sha256((binary_sha + digest(script)).encode()).hexdigest()
        statepath = root / 'legacy-archive-rollout-state.json'
        state = reserve(load(statepath, {'artifact': artifact, 'completed': [], 'in_flight': None}), args.host, artifact)
        save(statepath, state)
        stage = '/root/hashburst-archive-rollout/' + artifact
        job = 'hb-legacy-archive-' + artifact[:20]
        ssh('install -d -m 700 ' + stage)
        # Immutable, content-addressed staging. Identical uploads are safe before job submission.
        for path in [script, binary, *[work / name for name in PINS]]:
            destination = 'rollout-archive.py' if path == script else path.name
            exists = ssh('test ! -e ' + stage + '/' + destination + ' || sha256sum ' + stage + '/' + destination).strip()
            if exists:
                require(exists.split()[0] == digest(path), 'remote staged artifact differs')
            else:
                pending = stage + '/' + destination + '.upload'
                scp(str(path), 'root@' + args.host + ':' + pending)
                require(ssh('sha256sum ' + pending).split()[0] == digest(path), 'upload digest mismatch')
                ssh('mv ' + pending + ' ' + stage + '/' + destination)
        command = ['/usr/bin/python3', stage + '/rollout-archive.py', '--worker', '--stage', stage, '--sha256', binary_sha]
        status = json.loads(ssh('python3 -c ' + shlex.quote(
            'import json,pathlib; p=pathlib.Path(' + repr(stage + '/state.json') + '); print(p.read_text() if p.exists() else "{}")')))
        if status.get('phase') != 'verified':
            jobstate = ssh('systemctl show ' + job + '.service -p ActiveState --value').strip()
            if jobstate not in ('active', 'activating'):
                ssh('systemctl reset-failed ' + job + '.service 2>/dev/null || true')
                ssh(shlex.join(['systemd-run', '--no-block', '--collect', '--unit=' + job,
                    '--property=Type=oneshot', '--property=TimeoutStartSec=360', *command]))
            poll = '''import json,pathlib,subprocess,time
p=pathlib.Path(%r)
for _ in range(110):
 s=json.loads(p.read_text()) if p.exists() else {}
 print('ARCHIVE_PHASE='+s.get('phase','pending'),flush=True)
 if s.get('phase')=='verified': break
 job=subprocess.check_output(['systemctl','show',%r,'-p','ActiveState','--value'],text=True).strip()
 if job not in ('active','activating'): raise SystemExit('Worker stopped; inspect journal; originals retained, no writable rollback')
 time.sleep(3)
else: raise SystemExit('Deadline; persistent job retained; rerun same command')
''' % (stage + '/state.json', job + '.service')
            run('ssh', *SSH_OPTIONS, 'root@' + args.host, 'python3 -c ' + shlex.quote(poll), timeout=370)
        # Always fresh verification: a cached acceptance cannot authorize the next host.
        print(ssh(shlex.join(command + ['--check-only'])), end='')
        reportpath = work / ('ACCEPTANCE-' + args.host + '.json')
        scp('root@' + args.host + ':' + stage + '/ACCEPTANCE.json', str(reportpath))
        report = load(reportpath)
        require(report['binary_sha256'] == binary_sha and report['managed_host_archive_verified'], 'acceptance mismatch')
        if args.host not in state['completed']:
            state['completed'].append(args.host)
        state['in_flight'] = None
        save(statepath, state)
        print('ONE_LEGACY_ARCHIVE_VERIFIED=' + args.host)
        print('REPORT=' + str(reportpath))
        print('MAINNET_NOT_IMPORTED; fleet-wide freeze audit still required')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--host', choices=HOSTS)
    p.add_argument('--deployer-root')
    p.add_argument('--worker', action='store_true', help=argparse.SUPPRESS)
    p.add_argument('--check-only', action='store_true', help=argparse.SUPPRESS)
    p.add_argument('--stage', help=argparse.SUPPRESS)
    p.add_argument('--sha256', help=argparse.SUPPRESS)
    a = p.parse_args()
    if a.worker:
        worker(Path(a.stage), a.sha256, a.check_only)
    else:
        require(a.host and a.deployer_root, '--host and --deployer-root required')
        local(a)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print('STOP: ' + str(exc) + '; state retained; no writable rollback', file=sys.stderr)
        sys.exit(1)
