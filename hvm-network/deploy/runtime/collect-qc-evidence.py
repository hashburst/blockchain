#!/usr/bin/env python3
"""Collect public validator sets for recorded commitments; never restart nodes.

Run on the administration Mac. Input commitments are the already accepted
five-node references, not newly trusted roots returned by the same RPC.
"""
import argparse
import fcntl
import importlib.util
import json
from pathlib import Path


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--deployer-root', type=Path, required=True)
    parser.add_argument('--audit-directory', type=Path, required=True)
    args = parser.parse_args()
    root = args.deployer_root.resolve(strict=True)
    directory = args.audit_directory.resolve(strict=True)
    runtime = Path(__file__).resolve().parent
    d = load('existing_deployer', root / 'hashburst_deployer.py')
    pre = load('qc_preflight', runtime.parent / 'testnet/apow/preflight.py')
    rewards = json.loads((directory / 'rewards.json').read_text())
    commitments = [r['commitment'] for r in rewards['results']]
    validator = json.loads((directory / 'validator-v4-restart.json').read_text())
    commitments.append(validator['new_finalized_precommit']['commitment'])
    fleet = json.loads((directory / 'fleet-after-v4-restart.json').read_text())
    commitments.extend(fleet['commitments'])
    if len(rewards['results']) != 64 or validator['phase'] != 'verified':
        raise RuntimeError('acceptance references missing')
    source = pre.SOURCE + '''
result=[]
for commitment in p['commitments']:
 response=get('/rpc',json.dumps({'jsonrpc':'2.0','id':1,'method':'hb_getValidatorSet','params':[commitment['height']]}).encode())
 if response.get('error'):raise RuntimeError(str(response['error']))
 value=response['result']
 if value['height']!=commitment['height'] or value['validator_set_root']!=commitment['validator_set_root']:
  raise RuntimeError('historical validator set unavailable or root differs; require authenticated replay export')
 result.append(dict(chain_id=commitment['chain_id'],height=commitment['height'],hash=commitment['hash'],validator_set_root=commitment['validator_set_root'],certificate=commitment['certificate'],set={k:value[k] for k in ('height','validators','power')}))
print('HB_RESULT='+json.dumps(result))
'''
    with (root / 'deployer.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        for host, node in pre.TARGETS:
            evidence = d.remote(host, source, {'commitments': commitments})
            d.save(directory / ('qc-evidence-' + node + '.json'), evidence)
    print('QC_EVIDENCE_COLLECTED_READ_ONLY: run hvm-qc-audit on each file')


if __name__ == '__main__':
    main()
