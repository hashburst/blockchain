#!/usr/bin/env python3
"""Prompt locally on the key-holding host. Never pass passwords via argv or env."""
import argparse
import getpass
import subprocess
import sys

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--binary', required=True)
p.add_argument('--keystore', required=True)
p.add_argument('--address', required=True)
a = p.parse_args()
if not sys.stdin.isatty():
    raise SystemExit('STOP: interactive terminal required (SSH: use -t)')
password = getpass.getpass('Password del keystore (non SSH, non salvata): ')
if '\n' in password or '\r' in password or len(password.encode()) > 4095:
    raise SystemExit('STOP: unsupported password input')
result = subprocess.run([a.binary, '--keystore', a.keystore, '--address', a.address],
                        input=password + '\n', text=True)
raise SystemExit(result.returncode)
