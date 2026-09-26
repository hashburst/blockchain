#!/usr/bin/env python3
"""Read-only readiness check. Prints public TEP identity, never a secret file."""
import ipaddress
import json
import os
from pathlib import Path
import platform
import socket
import subprocess
import sys
import urllib.request


def check(expected_ip):
    ipaddress.IPv4Address(expected_ip)
    report = {'schema': 1, 'expected_ip': expected_ip, 'hostname': socket.gethostname(),
              'architecture': platform.machine(), 'errors': []}
    errors = report['errors']
    if platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'amd64'):
        errors.append('Linux amd64 required')
    if os.geteuid() != 0:
        errors.append('SSH root required for service/path checks')
    result = subprocess.run(['ip', '-j', '-4', 'address', 'show'], text=True, capture_output=True, timeout=5)
    if result.returncode:
        errors.append('Cannot verify local IPv4 addresses')
    else:
        local = [a['local'] for interface in json.loads(result.stdout) for a in interface.get('addr_info', []) if a.get('family') == 'inet']
        report['expected_ip_present'] = expected_ip in local
        if expected_ip not in local:
            errors.append('Expected IPv4 not assigned locally; verify NAT/host mapping before proceeding')
    result = subprocess.run(['systemctl', 'is-active', 'hashburst-tep.service'], text=True, capture_output=True, timeout=5)
    report['tep_service'] = result.stdout.strip()
    if result.returncode or report['tep_service'] != 'active':
        errors.append('TEP service not active')
    try:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open('http://127.0.0.1:47778/', timeout=5) as response:
            body = response.read(1048577)
        if len(body) > 1048576:
            raise ValueError('TEP response exceeds limit')
        tep = json.loads(body)
        key = tep.get('pubkey', '')
        valid = isinstance(key, str) and len(key) == 64
        if valid:
            try:
                decoded = bytes.fromhex(key)
                valid = len(decoded) == 32 and any(decoded)
            except ValueError:
                valid = False
        report['tep_node_id'] = tep.get('node_id')
        report['tep_app_ready'] = tep.get('app_ready') is True
        report['tep_peers_online'] = sum(p.get('online') is True for p in tep.get('peers', []))
        if valid:
            report['tep_public_key'] = key.lower()
        else:
            errors.append('Missing/invalid TEP public X25519 key')
        if not report['tep_node_id'] or not report['tep_app_ready']:
            errors.append('TEP authenticated application identity not ready')
        if report['tep_peers_online'] == 0:
            errors.append('No online TEP peer observed')
    except Exception as exc:
        errors.append('TEP public status unavailable: ' + type(exc).__name__)
    report['tcp_ports_free'] = {}
    for port in (18009, 31307):
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            try:
                sock.bind(('0.0.0.0', port))
                report['tcp_ports_free'][str(port)] = True
            except OSError:
                report['tcp_ports_free'][str(port)] = False
                errors.append(f'TCP port {port} unavailable')
    paths = ('/etc/hashburst-hvm-testnet', '/var/lib/hashburst-hvm-testnet', '/root/hvm-testnet-identity')
    report['existing_paths'] = [p for p in paths if Path(p).exists() or Path(p).is_symlink()]
    if report['existing_paths']:
        errors.append('Existing testnet paths: preserve and inspect; do not overwrite')
    for name in ('hashburst-hvm-testnet.service', 'hashburst-hvm-testnet-canary.service'):
        result = subprocess.run(['systemctl', 'show', name, '-p', 'ActiveState', '--value'], text=True, capture_output=True, timeout=5)
        if result.stdout.strip() in ('active', 'activating', 'reloading'):
            errors.append('Existing running unit: ' + name)
    report['ok'] = not errors
    return report


if __name__ == '__main__':
    try:
        if len(sys.argv) != 2:
            raise ValueError('expected IPv4 argument required')
        report = check(sys.argv[1])
    except Exception as exc:
        report = {'ok': False, 'errors': ['Preflight failed: ' + type(exc).__name__]}
    print(json.dumps(report, indent=2))
    sys.exit(0 if report['ok'] else 1)
