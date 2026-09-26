#!/usr/bin/env python3
"""Compare the native canary receipts and a fixed finalized height with the observer."""
import json, sys, time, urllib.request
from pathlib import Path
RPC = 'https://blockchainapi.one/api/hashburst/hvm/testnet/rpc'
def rpc(method, params):
    req = urllib.request.Request(RPC, data=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(), headers={'Content-Type':'application/json'})
    with urllib.request.urlopen(req, timeout=20) as response:
        data=json.load(response)
    if 'error' in data: raise RuntimeError(data['error'])
    return data['result']
def verify(proof):
    if proof['chain_id'] != 4735490: raise ValueError('wrong proof chain')
    if int(rpc('eth_chainId', []),16) != 4735490: raise ValueError('wrong public chain')
    for key in ('deploy_receipt','call_receipt'):
        expected=proof[key]
        actual=rpc('hb_getTransactionReceipt', ['0x'+expected['txid'].removeprefix('0x')])
        if actual != expected: raise RuntimeError(key+' not yet identical on observer')
    expected=proof['commitment']; actual=rpc('hb_getFinalizedCommitment',[expected['height']])
    for key in ('chain_id','height','hash','parent_hash','hbt_state_root','hvm_state_root','receipts_root','validator_set_root'):
        if actual[key] != expected[key]: raise RuntimeError('commitment mismatch: '+key)
    if not actual.get('certificate'): raise RuntimeError('certificate absent')
    print('PUBLIC_NATIVE_RECEIPTS_AGREEMENT_OK')
    print('PUBLIC_NATIVE_FINALIZED_COMMITMENT_OK height='+str(expected['height']))
    print('HVM_NATIVE_FUNDED_APPLICATION_TEST_OK')
    print('EVM_METAMASK_MAINNET_NOT_CERTIFIED')
if __name__=='__main__':
    proof=json.loads(Path(sys.argv[1]).read_text())
    end=time.monotonic()+180
    while True:
        try: verify(proof); break
        except Exception as exc:
            if time.monotonic()>=end: raise SystemExit('STOP: '+str(exc))
            print('WAIT_PUBLIC_OBSERVER:',exc,flush=True);time.sleep(5)
