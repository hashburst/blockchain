#!/usr/bin/env python3
"""Create/check one offline terminal generation. Never change a running service."""
import argparse
import getpass
import json
import pathlib
import subprocess
import sys

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--close-binary',required=True)
p.add_argument('--archive-binary',required=True)
p.add_argument('--source',required=True)
p.add_argument('--out',required=True)
p.add_argument('--keystore',required=True)
a=p.parse_args()
out=pathlib.Path(a.out)
if not out.exists():
    subprocess.run([a.close_binary,'--source',a.source,'--check-source'],check=True)
    if not sys.stdin.isatty():
        raise SystemExit('STOP: interactive terminal required (SSH -t)')
    print('OPERAZIONE: firma terminale indice 10; 450 HBT al destinatario fissato; emissione zero.')
    print('Viene creata solo una copia separata. Nessun invio alla rete o modifica dei servizi.')
    password=getpass.getpass('Password del keystore legacy (non salvata): ')
    if '\n' in password or '\r' in password or len(password.encode())>4095:
        raise SystemExit('STOP: unsupported password input')
    subprocess.run([a.close_binary,'--source',a.source,'--out',a.out,
        '--keystore',a.keystore,'--authorize-terminal-transfer-450'],
        input=password+'\n',text=True,check=True)
else:
    print('EXISTING_GENERATION: verifying; no second signature')
manifest=out/'manifest.json'
if manifest.is_symlink() or not manifest.is_file() or manifest.stat().st_size>65536:
    raise SystemExit('STOP: invalid manifest; retain generation for inspection')
m=json.loads(manifest.read_text())
if (m.get('schema')!='hashburst-legacy-terminal-candidate-v1' or
    m.get('source_chain_id')!=1337 or m.get('target_chain_id')!=4735489 or
    m.get('imported_units')!=45000000000 or m.get('new_issuance_units')!=0 or
    m.get('activation_allowed') is not False):
    raise SystemExit('STOP: unexpected candidate manifest')
subprocess.run([a.archive_binary,'--directory',a.out,'--check',
    '--dat-sha256',m['file_sha256']['blockchain.dat'],
    '--idx-sha256',m['file_sha256']['blockchain.idx'],
    '--terminal-hash',m['terminal_hash']],check=True)
print(json.dumps(m,indent=2))
print('TERMINAL_GENERATION_VERIFIED_NO_FLEET_FREEZE_NO_MAINNET_IMPORT')
