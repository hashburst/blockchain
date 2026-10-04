#!/usr/bin/env python3
"""Locate declared wallet addresses; never print, decrypt or copy private key material."""
import argparse
import datetime
import json
import os
import pathlib
import re
import stat
import subprocess
import time

ROOTS = ['/root/node-keystore', '/root/keystore', '/root/.ethereum/keystore',
         '/var/lib/hashburst', '/etc/hashburst', '/root/hashburst-wallets']
SKIP = {'.git', 'node_modules', '__pycache__', '.cache', 'ipfs', '.ipfs', 'blocks', 'chainstate'}
RAW_NAMES = {'wallet.key', 'private.key', 'privatekey', 'private_key', 'privatekey.hex'}

def address(value):
    if not isinstance(value, str):
        return None
    value = value.lower()
    if value.startswith('0x'):
        value = value[2:]
    return '0x' + value if re.fullmatch('[0-9a-f]{40}', value) else None


def scan(roots, targets, max_files=20000, seconds=30):
    targets = set(targets)
    result = {'matches': [], 'unattributed_key_files': [], 'roots': [], 'errors': [],
              'files_examined': 0, 'bounded_scan': True, 'limit_reached': False,
              'private_material_exported': False, 'ownership_proven': False}
    seen = set()
    deadline = time.monotonic() + seconds
    for name in roots:
        root = pathlib.Path(name).expanduser()
        if root.is_symlink():
            result['roots'].append({'path': str(root), 'status': 'symlink_skipped'})
            continue
        if not root.is_dir():
            result['roots'].append({'path': str(root), 'status': 'absent_or_inaccessible'})
            continue
        result['roots'].append({'path': str(root), 'status': 'searched'})
        def walk_error(exc):
            result['errors'].append({'path': str(exc.filename), 'error': 'directory_unreadable'})
        for current, dirs, files in os.walk(root, followlinks=False, onerror=walk_error):
            depth = len(pathlib.Path(current).relative_to(root).parts)
            dirs[:] = sorted(d for d in dirs if d not in SKIP and not d.startswith('.')
                             and not pathlib.Path(current, d).is_symlink()) if depth < 6 else []
            for filename in sorted(files):
                if time.monotonic() > deadline or result['files_examined'] >= max_files:
                    result['limit_reached'] = True
                    return result
                result['files_examined'] += 1
                path = pathlib.Path(current, filename)
                try:
                    st = path.lstat()
                    if not stat.S_ISREG(st.st_mode) or (st.st_dev, st.st_ino) in seen:
                        continue
                    seen.add((st.st_dev, st.st_ino))
                    meta = {'path': str(path), 'mode': oct(stat.S_IMODE(st.st_mode)),
                            'uid': st.st_uid, 'bytes': st.st_size}
                    if filename.lower() in RAW_NAMES:
                        result['unattributed_key_files'].append(meta)
                        continue  # Never read raw keys, even to derive an address.
                    if not (filename.startswith('UTC--') or filename.endswith('.json')
                            or filename.lower() in {'address', 'address.txt', 'wallet.address'}):
                        continue
                    if st.st_size > 262144:
                        continue
                    fd = os.open(str(path), os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0))
                    with os.fdopen(fd, 'rb') as stream:
                        opened = os.fstat(stream.fileno())
                        if (opened.st_dev, opened.st_ino) != (st.st_dev, st.st_ino):
                            continue
                        raw = stream.read(262145)
                    if len(raw) > 262144:
                        continue
                    try:
                        obj = json.loads(raw)
                    except (ValueError, UnicodeError):
                        obj = {'address': raw.decode(errors='replace').strip()} if filename.lower() in {'address', 'address.txt', 'wallet.address'} else None
                    if not isinstance(obj, dict):
                        continue
                    declared = address(obj.get('address'))
                    if declared not in targets:
                        continue
                    crypto = obj.get('crypto', obj.get('Crypto'))
                    encrypted = isinstance(crypto, dict) and isinstance(crypto.get('ciphertext'), str) and bool(crypto.get('kdf'))
                    result['matches'].append(dict(meta, declared_address=declared,
                        kind='encrypted_keystore_header' if encrypted else 'public_address_reference',
                        decryption_tested=False))
                except OSError:
                    result['errors'].append({'path': str(path), 'error': 'file_unreadable'})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hosts', nargs='+', required=True)
    parser.add_argument('--addresses', nargs='+', required=True)
    parser.add_argument('--remote-root', action='append', default=[])
    parser.add_argument('--local-directory', action='append', default=[])
    args = parser.parse_args()
    targets = [address(x) for x in args.addresses]
    if None in targets:
        parser.error('Every target must be a 20-byte hexadecimal address')
    for host in args.hosts:
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]*', host):
            parser.error('Invalid host')
    source = pathlib.Path(__file__).read_text()
    definitions = source[:source.index('\ndef main():')]
    payload = {'roots': ROOTS + args.remote_root, 'targets': targets}
    remote = definitions + '\npayload = json.loads(' + repr(json.dumps(payload)) + ')\nprint(json.dumps(scan(**payload)))\n'
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    output = pathlib.Path('wallet-location-' + stamp + '.json')
    report = {'targets': targets, 'created_at': stamp, 'hosts': {}, 'read_only': True}
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as file:
        def save():
            file.seek(0); json.dump(report, file, indent=2); file.write('\n'); file.truncate(); file.flush()
        save()
        for host in args.hosts:
            print('READ_ONLY=' + host, flush=True)
            try:
                p = subprocess.run(['ssh', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
                    '-o', 'ConnectTimeout=15', '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3',
                    '-o', 'PubkeyAuthentication=no', '-o', 'PreferredAuthentications=keyboard-interactive,password',
                    'root@' + host, 'python3 -'], input=remote, text=True,
                    stdout=subprocess.PIPE, timeout=180)
                result = json.loads(p.stdout) if p.returncode == 0 else {'ssh_exit': p.returncode}
                report['hosts'][host] = result
                print('DECLARED_ADDRESS_MATCHES=' + str(len(result.get('matches', []))), flush=True)
            except Exception as exc:
                report['hosts'][host] = {'error': type(exc).__name__}
            save()
        if args.local_directory:
            report['local'] = scan(args.local_directory, targets)
            save()
    print('REPORT=' + str(output.resolve()))
    print('NO_KEYS_EXPORTED_NO_SERVICE_CHANGED')


if __name__ == '__main__':
    main()
