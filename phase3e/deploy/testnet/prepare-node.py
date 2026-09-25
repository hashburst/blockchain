#!/usr/bin/env python3
"""Install a new offline bootstrap, pin state, leave the default role observer.
Never starts/enables a service. Refuses existing paths; retains partial state on error.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import pwd
import shutil
import socket
import subprocess

RELEASE = '06d7c3bd2cff59b2e95b25ed09fe8509dd6de01a'
BINARY = Path('/opt/hashburst-hvm-testnet/releases') / RELEASE / 'hashburst-testnet'
ETC = Path('/etc/hashburst-hvm-testnet')
DATA = Path('/var/lib/hashburst-hvm-testnet')


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--host', required=True)
    p.add_argument('--bundle', required=True, type=Path)
    p.add_argument('--identity', required=True, type=Path)
    p.add_argument('--node-id', required=True)
    p.add_argument('--chain-id', required=True, type=int)
    p.add_argument('--digest', required=True)
    p.add_argument('--runtime-sha256', required=True)
    a = p.parse_args()
    require(os.geteuid() == 0 and socket.gethostname() == a.host, 'root / host mismatch')
    require(a.chain_id > 0 and a.chain_id != 1337, 'explicit non-legacy testnet ID required')
    require(a.node_id.isascii() and all(c.isalnum() or c in '_-' for c in a.node_id), 'invalid node ID')
    require(BINARY.is_file() and not BINARY.is_symlink(), 'stage the exact runtime release first')
    require(hashlib.sha256(BINARY.read_bytes()).hexdigest() == a.runtime_sha256, 'runtime hash mismatch')
    for target in (ETC, DATA):
        require(not target.exists() and not target.is_symlink(), f'existing destination refused: {target}')
        require(target.parent.resolve() == target.parent, 'symlink destination parent refused')
    bundle = a.bundle.resolve(strict=True)
    identity = a.identity.resolve(strict=True)
    require((bundle / 'BOOTSTRAP_COMPLETE').read_text().strip() == a.digest, 'incomplete or wrong bundle')
    seen = set()
    for line in (bundle / 'SHA256SUMS').read_text().splitlines():
        digest, rel = line.split('  ', 1)
        file = bundle / rel
        require(not Path(rel).is_absolute() and '..' not in Path(rel).parts, 'unsafe manifest path')
        require(file.resolve().is_relative_to(bundle) and not file.is_symlink(), 'unsafe bundle member')
        require(rel not in seen, 'duplicate manifest member')
        seen.add(rel)
        require(hashlib.sha256(file.read_bytes()).hexdigest() == digest, f'checksum mismatch: {rel}')
    needed = {'network.json', 'BOOTSTRAP_COMPLETE', 'checkpoint/blockchain.dat',
              'checkpoint/blockchain.idx', a.node_id + '.observer.json', a.node_id + '.validator.json'}
    require(needed <= seen, 'missing checksummed bootstrap files')
    network = json.loads((bundle / 'network.json').read_text())
    require(network['chain_id'] == a.chain_id and network['config_digest'] == a.digest, 'network mismatch')
    observer = json.loads((bundle / (a.node_id + '.observer.json')).read_text())
    validator = json.loads((bundle / (a.node_id + '.validator.json')).read_text())
    public = json.loads((identity / 'public.json').read_text())
    registration = json.loads(public['registration']['Data'])
    require(registration['node_id'] == a.node_id and registration['peer_id'] == observer['peer_id'], 'local identity mismatch')
    require(observer['role'] == 'observer' and not observer['consensus_key_file'], 'observer config required')
    for c in (observer, validator):
        require(c['node_id'] == a.node_id and c['protocol']['chain_id'] == a.chain_id, 'config identity mismatch')
        require(c['data_dir'] == str(DATA) and c['p2p_key_file'] == str(ETC / 'p2p.key'), 'unexpected paths')
    require(validator['consensus_key_file'] == str(ETC / 'consensus.key') and validator['role'] == 'validator', 'invalid validator config')
    for name in ('p2p.key', 'consensus.key'):
        key = identity / name
        require(key.is_file() and not key.is_symlink() and key.stat().st_mode & 0o077 == 0, 'key permissions invalid')
    try:
        user = pwd.getpwnam('hashburst-hvm-testnet')
    except KeyError:
        subprocess.run(['useradd', '--system', '--user-group', '--no-create-home', '--shell', '/usr/sbin/nologin', 'hashburst-hvm-testnet'], check=True)
        user = pwd.getpwnam('hashburst-hvm-testnet')
    require(user.pw_uid != 0, 'invalid service user')
    os.umask(0o077)
    for target in (ETC, DATA):
        target.mkdir(mode=0o700)
        os.chown(target, user.pw_uid, user.pw_gid)
    def copy(source, target):
        with target.open('xb') as output:
            with source.open('rb') as handle:
                shutil.copyfileobj(handle, output)
            output.flush()
            os.fsync(output.fileno())
        os.chmod(target, 0o600)
        os.chown(target, user.pw_uid, user.pw_gid)
    for name in ('blockchain.dat', 'blockchain.idx'):
        copy(bundle / 'checkpoint' / name, DATA / name)
    for name in ('p2p.key', 'consensus.key'):
        copy(identity / name, ETC / name)
    copy(bundle / (a.node_id + '.observer.json'), ETC / 'node.json')
    copy(bundle / (a.node_id + '.validator.json'), ETC / 'validator.json')
    for config, flag in [('node.json', '--provision'), ('node.json', '--check'), ('validator.json', '--check')]:
        subprocess.run(['runuser', '-u', user.pw_name, '--', str(BINARY), '--config', str(ETC / config), flag], check=True)
    require((DATA / 'runtime.pin').read_text().splitlines()[0] == a.digest, 'runtime digest mismatch')
    print('HVM_TESTNET_PERSISTENT_STATE_PREPARED role=observer')
    print('VALIDATOR_KEY_AND_REGISTRY_CHECK_OK')
    print('NO_SERVICE_STARTED')


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        raise SystemExit(f'PREPARE_FAILED: {exc}; partial files retained, no service started')
