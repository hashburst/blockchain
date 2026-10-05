"""Offline economic model. NOT a consensus handler or production state writer."""
import copy
import hashlib
import json

RECIPIENT = '0xd1da8d04d767685e53440dbc56803af350a65333'
TERMINAL = '0000a8bef0916f373a97fa8c311b095258f00cf7f2a24a3ff2f875156ca27ae1'
UNITS = 45000000000
FOUNDER = 100000000000000000
SOURCE_ID = hashlib.sha256(('HashBurst/legacy-import/v1:1337:10:' + TERMINAL).encode()).hexdigest()

def apply_import(state, manifest):
    """Pure transition: no mutation on rejection; source-bound replay protection.

    Production must commit consumed sources together with balances in the
    consensus state root and restore BOTH from the authenticated checkpoint.
    """
    if state.get('chain_id') != 4735489:
        raise ValueError('wrong target chain')
    expected = {'source_chain_id': 1337, 'source_height': 10, 'source_hash': TERMINAL,
                'target_chain_id': 4735489, 'recipient': RECIPIENT, 'amount_units': str(UNITS),
                'source_nullifier': SOURCE_ID, 'additional_migration_issuance_units': '0'}
    if any(manifest.get(k) != v for k, v in expected.items()):
        raise ValueError('migration commitment differs')
    if SOURCE_ID in state['consumed_sources']:
        raise ValueError('source already imported')
    result = copy.deepcopy(state)
    before = sum(int(v) for v in result['balances_units'].values())
    amount = int(result['balances_units'].get(RECIPIENT, '0')) + UNITS
    if amount > 2**63 - 1:
        raise ValueError('native balance overflow')
    result['balances_units'][RECIPIENT] = str(amount)
    result['consumed_sources'].append(SOURCE_ID)
    if sum(int(v) for v in result['balances_units'].values()) != before + UNITS:
        raise ValueError('reconciliation failed')
    return result

def state_commitment(state):
    return hashlib.sha256(json.dumps(state, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
