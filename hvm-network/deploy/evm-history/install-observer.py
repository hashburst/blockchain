#!/usr/bin/env python3
"""Update the existing non-signing testnet observer; never provision or migrate."""
import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import time
import urllib.request
from pathlib import Path

SERVICE = 'hashburst-hvm-testnet-ingress.service'
CONFIG = Path('/etc/hashburst-hvm-testnet-ingress/node.json')
DATA = Path('/var/lib/hashburst-hvm-testnet-ingress')
ROOT = Path('/opt/hashburst-hvm-testnet-ingress')
HEALTH = 'http://127.0.0.1:18009/health'
NAMES = ('runtime.pin', 'consensus-votes.jsonl', 'consensus-bft-signatures.jsonl')
OLD_BINARY = '1e52296f36b6666d5fc2d4f7ab1d312d7ded467c73312be946d65211eaac8ba9'

def require(value, message):
    if not value:
        raise RuntimeError(message)

def sha(path):
    with open(path, 'rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest() if hasattr(hashlib, 'file_digest') else hashlib.sha256(f.read()).hexdigest()

def run(*args):
    return subprocess.run(args, check=True, text=True, capture_output=True, timeout=180).stdout.strip()

def properties():
    text = run('systemctl', 'show', SERVICE, '--no-pager', '--property=LoadState,ActiveState,MainPID,ExecStart,User,NoExecPaths,ExecPaths,RootDirectory,RootImage')
    return dict(line.split('=', 1) for line in text.splitlines() if '=' in line)

def health():
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(HEALTH, timeout=10) as r:
        return json.load(r)

def validate_config(c):
    require((c.get('node_id'), c.get('role'), c.get('network'), c.get('protocol', {}).get('chain_id')) == ('hvm-testnet-ingress', 'observer', 'testnet', 4735490), 'Unexpected observer identity or network')
    require(not c.get('consensus_key_file') , 'Observer signing configuration must be empty')
    require(c.get('data_dir') == str(DATA) and c.get('rpc_listen') == '127.0.0.1:18009', 'Unexpected observer paths/listener')
    require(c.get('protocol', {}).get('evm') is not None, 'EVM configuration absent; no migration performed')

def file_proofs():
    return {name: sha(DATA / name) if (DATA / name).exists() else None for name in NAMES}

def preserved(proof):
    require(sha(CONFIG) == proof['config_sha256'], 'Configuration changed')
    require(file_proofs() == proof['files'], 'Observer pin or journal changed')

def check_health(h, proof):
    require((h.get('chain_id'), h.get('role'), h.get('node_id')) == (4735490, 'observer', 'hvm-testnet-ingress'), 'Wrong live identity')
    require(h.get('peer_id') == proof['peer_id'] and h.get('config_digest') == proof['config_digest'], 'Live identity/config digest changed')
    return h.get('ok') is True and h.get('peer_count', 0) > 0 and h.get('finalized_height', 0) > proof['finalized_before']

def wait_ready(release, timeout):
    proof = json.loads((release / 'proof-before.json').read_text())
    binary = release / 'hashburst-testnet'
    preserved(proof)
    deadline = time.monotonic() + timeout
    last_message = None
    while time.monotonic() < deadline:
        props = properties()
        if props.get('ActiveState') == 'failed':
            raise RuntimeError('Observer failed; inspect journalctl, do not restart repeatedly')
        try:
            h = health()
        except (OSError, ValueError) as e:
            message = type(e).__name__ + ': replay/startup not yet ready'
        else:
            if check_health(h, proof):
                pid = int(props.get('MainPID', '0'))
                require(pid > 0 and sha(f'/proc/{pid}/exe') == sha(binary), 'Running binary differs')
                preserved(proof)
                run('python3', str(release / 'verify-history.py'), '--url', 'http://127.0.0.1:18009/evm', '--out', str(release / 'GATE-local-history.json'))
                (release / 'complete.json').write_text(json.dumps({'ok': True, 'health': h, 'binary_sha256': sha(binary)}, indent=2))
                print('OBSERVER_HISTORY_API_READY', flush=True)
                print('OBSERVER_IDENTITY_CONFIG_PIN_JOURNALS_PRESERVED', flush=True)
                return
            message = f"catch-up finalized={h.get('finalized_height')} peers={h.get('peer_count')}"
        if message != last_message:
            print('WAIT ' + message, flush=True)
            last_message = message
        time.sleep(15)
    raise RuntimeError('Readiness deadline reached; state retained. Use verify, not another restart')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=('install', 'verify'))
    parser.add_argument('--timeout', type=int, default=7200)
    args = parser.parse_args()
    require(os.geteuid() == 0, 'Run on observer as root')
    package = Path(__file__).resolve().parent
    source = package / 'hashburst-testnet'
    manifest = json.loads((package / 'release.json').read_text())
    require(sha(source) == manifest['binary_sha256'], 'Package binary checksum mismatch')
    validate_config(json.loads(CONFIG.read_text()))
    require(not DATA.is_symlink() and not ROOT.is_symlink(), 'Unexpected state/release symlink')
    release = ROOT / ('rpc-history-' + manifest['binary_sha256'][:16])
    marker = release / 'proof-before.json'
    if args.action == 'verify' or marker.exists():
        require(marker.exists(), 'No installed release proof')
        print('VERIFY_ONLY_NO_RESTART', flush=True)
        wait_ready(release, args.timeout)
        return
    props = properties()
    require(props.get('LoadState') == 'loaded' and props.get('ActiveState') == 'active', 'Observer must be active before update')
    require(props.get('User') not in ('', 'root'), 'Dedicated service user required')
    require(not any(props.get(k) for k in ('RootDirectory', 'RootImage', 'NoExecPaths', 'ExecPaths')), 'Execution path restriction needs an explicit compatible release path; hardening retained')
    pid = int(props['MainPID'])
    require(pid > 0 and sha(f'/proc/{pid}/exe') == OLD_BINARY, 'Unexpected baseline binary; no service changed')
    before = health()
    proof = {'config_sha256': sha(CONFIG), 'files': file_proofs(), 'peer_id': before.get('peer_id'), 'config_digest': before.get('config_digest'), 'finalized_before': before.get('finalized_height'), 'old_exec_start': props['ExecStart']}
    check_health(before, proof)
    require(before.get('ok') is True and before.get('peer_count', 0) > 0, 'Observer not healthy')
    require(proof['files']['runtime.pin'] is not None, 'Runtime pin missing')
    release.mkdir(mode=0o755)
    for name in ('hashburst-testnet', 'verify-history.py'):
        shutil.copyfile(package / name, release / name)
        os.chmod(release / name, 0o755)
    # Probe executable access before stopping the observer. No config or keys loaded.
    run('runuser', '-u', props['User'], '--', str(release / 'hashburst-testnet'), '--help')
    marker.write_text(json.dumps(proof, indent=2))
    os.chmod(marker, 0o600)
    (release / 'unit-before.txt').write_text(run('systemctl', 'cat', SERVICE))
    os.chmod(release / 'unit-before.txt', 0o600)
    override = Path('/etc/systemd/system') / (SERVICE + '.d') / '99-historical-rpc.conf'
    require(not override.exists(), 'Release override already exists; inspect before changing')
    run('systemctl', 'stop', SERVICE)
    require(properties().get('MainPID') == '0', 'Observer still running')
    preserved(proof)
    override.parent.mkdir(mode=0o755, exist_ok=True)
    override.write_text('[Service]\nExecStart=\nExecStart=' + str(release / 'hashburst-testnet') + ' --config ' + str(CONFIG) + '\n')
    run('systemctl', 'daemon-reload')
    require(str(release / 'hashburst-testnet') in properties().get('ExecStart', ''), 'Effective executable override not selected')
    run('systemctl', 'start', SERVICE)
    print('OBSERVER_UPDATE_STARTED_RELEASE=' + str(release), flush=True)
    wait_ready(release, args.timeout)

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit('STOP: ' + str(error) + '. State and configuration retained; no migration or automatic rollback.')
