#!/usr/bin/env python3
"""Read-only fleet acceptance for the evidenced 2026-10-05 HA transition.
Does not modify the pinned installer, protected.json, or rollout state.
"""
import argparse
import hashlib
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request

ARTIFACT = '741fcf6003679c4c9d7f1998f4706bc560481b84253828a20d4f0f140bdaeb86'
INSTALLER_SHA = '563be4a6b9acee0f1fd11502b47679ae7fdad3d0eb2cee3494e27d28789af5dc'
BINARY_SHA = '0f01f861824b60f84bb4212f3eabb72f0eed0a284bd94aef2aebdc451e6f0eda'
HOSTS = ('64.31.4.9', '77.90.188.157', '77.90.188.155', '77.90.188.154', '77.90.188.153')
MASTER = 'hashburst-master.service'
SSH = ['-o', 'ControlMaster=no', '-o', 'ControlPath=none', '-o', 'ConnectTimeout=15',
       '-o', 'ServerAliveInterval=30', '-o', 'ServerAliveCountMax=3',
       '-o', 'PubkeyAuthentication=no', '-o', 'PreferredAuthentications=keyboard-interactive,password']


def require(value, message):
    if not value:
        raise RuntimeError(message)


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def classify(host, before, after):
    changed = [key for key in sorted(set(before) | set(after)) if before.get(key) != after.get(key)]
    if not changed:
        return False
    require(host == '77.90.188.153' and changed == [MASTER], 'unreviewed protected service difference')
    require(before.get(MASTER) == {'ActiveState': 'active', 'MainPID': '79801'}, 'unexpected original master')
    require(after.get(MASTER) == {'ActiveState': 'active', 'MainPID': '3206610'}, 'master changed again; review required')
    return True


def journal_evidence():
    raw = subprocess.check_output(['journalctl', '--utc', '-u', 'hashburst-ha-agent.service',
        '-u', 'hashburst-ha-watchdog.service', '--since', '2026-10-05 16:24:00 UTC',
        '--until', '2026-10-05 16:28:00 UTC', '--no-pager', '-o', 'json'], text=True, timeout=30)
    expected = ('Fencing primary-only service ' + MASTER,
                'Starting primary-only service ' + MASTER, 'HA PRIMARY acquired term=2 grants=3/3')
    matched = []
    for line in raw.splitlines():
        entry = json.loads(line)
        message = entry.get('MESSAGE', '')
        if isinstance(message, str) and any(item in message for item in expected):
            matched.append({'unit': entry.get('_SYSTEMD_UNIT'), 'timestamp_us': entry.get('__REALTIME_TIMESTAMP'),
                            'message': message})
    ordered = [next((int(e['timestamp_us']) for e in matched if item in e['message']), None) for item in expected]
    require(all(t is not None for t in ordered) and ordered == sorted(ordered), 'HA transition journal incomplete')
    return matched


def ha_status(config):
    host = config.get('bind_host') or '127.0.0.1'
    require(ipaddress.ip_address(host).is_loopback, 'HA status must use loopback')
    port = int(config.get('bind_port', 47780))
    require(1 <= port <= 65535, 'invalid HA port')
    address = '[' + host + ']' if ':' in host else host
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open('http://%s:%s/v1/status' % (address, port), timeout=15) as response:
        payload = json.load(response)
    s = payload.get('status', {})
    require(payload.get('ok') is True and s.get('armed') is True and s.get('eligible') is True,
            'HA not armed/eligible')
    require(s.get('local_role') == 'primary' and s.get('leader_remaining_ms', 0) > 0, 'HA has no current primary lease')
    return {k: s.get(k) for k in ('armed', 'eligible', 'local_role', 'leader_term', 'leader_remaining_ms', 'quorum')}


def remote(host):
    require(host in HOSTS, 'invalid host')
    stage = Path('/root/hashburst-archive-rollout') / ARTIFACT
    script = stage / 'rollout-archive.py'
    require(sha(script) == INSTALLER_SHA, 'pinned installer differs')
    # Avoid writing __pycache__ on the VPS.
    sys.dont_write_bytecode = True
    spec = importlib.util.spec_from_file_location('pinned_archive', script)
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    before_hash = sha(stage / 'protected.json')
    before = json.loads((stage / 'protected.json').read_text())
    after = m.protected()
    transition = classify(host, before, after)
    evidence = None
    if transition:
        config = json.loads(Path('/etc/hashburst/ha.json').read_text())
        require(config.get('armed') is True and config.get('primary_services') == [MASTER], 'HA service policy differs')
        require('hashburst-node.service' in config.get('required_services', []) and
                'hashburst-tep.service' in config.get('required_services', []), 'HA prerequisites differ')
        evidence = {'journal': journal_evidence(), 'current_ha': ha_status(config),
                    'before': before[MASTER], 'after': after[MASTER]}
    directory = Path('/opt/hashburst-legacy-archive') / BINARY_SHA
    require(m.UNIT_FILE.is_file() and not m.UNIT_FILE.is_symlink() and
            m.UNIT_FILE.read_bytes() == m.unit_text(directory), 'archive unit differs')
    props = m.properties(m.UNIT)
    require(props['ActiveState'] == 'active' and not props['DropInPaths'] and
            props['FragmentPath'] == str(m.UNIT_FILE), 'effective archive service differs')
    pid = int(props['MainPID'])
    require(pid > 0 and m.digest('/proc/%s/exe' % pid) == BINARY_SHA, 'running archive differs')
    m.check_pair(directory)
    m.no_old_writer()
    health = m.api_check()
    if transition:
        # Observe stability beyond the configured 12-second lease interval.
        time.sleep(15)
        evidence['second_ha'] = ha_status(config)
    require(m.protected() == after, 'protected services changed during acceptance')
    require(sha(stage / 'protected.json') == before_hash, 'baseline changed during audit')
    return {'host': host, 'observed_at_unix': int(time.time()), 'archive_verified': True,
            'binary_sha256': BINARY_SHA, 'health': health, 'file_sha256': m.PINS,
            'baseline_sha256': before_hash, 'protected_services_unchanged': not transition,
            'reviewed_ha_transition': evidence, 'remote_read_only': True}


def local(root):
    import fcntl
    root = Path(root).resolve(strict=True)
    with open(root / '.legacy-archive-rollout.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        statefile = root / 'legacy-archive-rollout-state.json'
        original = statefile.read_bytes()
        state = json.loads(original)
        require(state.get('artifact') == ARTIFACT and state.get('in_flight') == '77.90.188.153' and
                set(state.get('completed', [])) == set(HOSTS[:-1]), 'unexpected rollout state')
        reports = {}
        source = Path(__file__).read_text()
        for host in HOSTS:
            print('READ_ONLY_ARCHIVE_ACCEPTANCE=' + host, flush=True)
            result = subprocess.run(['ssh', *SSH, 'root@' + host, 'python3 - --remote ' + host],
                input=source, text=True, stdout=subprocess.PIPE, check=True, timeout=240)
            report = json.loads(result.stdout)
            require(report.get('host') == host and report.get('archive_verified') is True, 'invalid remote acceptance')
            reports[host] = report
        require(statefile.read_bytes() == original, 'rollout state changed during audit')
        out = root / 'legacy-archive-rollout' / ('FLEET-FREEZE-HA-' + str(time.time_ns()) + '.json')
        document = {'schema': 'hashburst-managed-legacy-freeze-ha-v1', 'artifact': ARTIFACT,
                    'fleet_freeze_verified': True, 'scope': 'five managed legacy services at observation times',
                    'reviewed_exception': 'HA-controlled master stop/start on .153 during archive replacement',
                    'original_rollout_state_sha256': hashlib.sha256(original).hexdigest(),
                    'original_rollout_state_preserved': True, 'mainnet_import_executed': False,
                    'activation_allowed': False, 'reports': reports}
        with open(out, 'x') as f:
            os.chmod(out, 0o600)
            json.dump(document, f, indent=2)
            f.write('\n')
            f.flush()
            os.fsync(f.fileno())
        print('FIVE_MANAGED_LEGACY_ARCHIVES_FROZEN_WITH_REVIEWED_HA_TRANSITION_OK')
        print('REPORT=' + str(out))
        print('OLD_ROLLOUT_STATE_PRESERVED; USE_THIS_REPORT; MAINNET_NOT_IMPORTED')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--deployer-root')
    parser.add_argument('--remote', choices=HOSTS, help=argparse.SUPPRESS)
    a = parser.parse_args()
    try:
        if a.remote:
            print(json.dumps(remote(a.remote)))
        else:
            require(a.deployer_root, '--deployer-root required')
            local(a.deployer_root)
    except Exception as e:
        print('STOP: ' + str(e) + '; no service or rollout state modified', file=sys.stderr)
        sys.exit(1)
