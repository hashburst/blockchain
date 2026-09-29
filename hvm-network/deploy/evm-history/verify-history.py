#!/usr/bin/env python3
"""Read-only fixed-height acceptance; no transaction submission or wallet access."""
import argparse
import json
import time
import urllib.request
from pathlib import Path

DEFAULT_URL = 'https://blockchainapi.one/api/hashburst/hvm/testnet/evm'
DEFAULT_ACCOUNT = '0xc4708173f7d276758a08b27821d98d94985dcdd1'

class RPC:
    def __init__(self, url):
        self.url, self.sequence = url, 0
    def call(self, method, params):
        self.sequence += 1
        req = urllib.request.Request(self.url, data=json.dumps(dict(jsonrpc='2.0', id=self.sequence, method=method, params=params)).encode(), headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(req, timeout=20) as response:
            result = json.load(response)
        if result.get('id') != self.sequence or 'error' in result or 'result' not in result:
            raise RuntimeError(f'{method}: {result}')
        time.sleep(0.25)
        return result['result']

def verify(url, account):
    rpc = RPC(url)
    if rpc.call('eth_chainId', []) != '0x484202':
        raise RuntimeError('Wrong chain: testnet 4735490 required')
    head = int(rpc.call('eth_blockNumber', []), 16)
    height = head - 1
    tag = hex(height)
    block = rpc.call('eth_getBlockByNumber', [tag, False])
    if not block or int(block['number'], 16) != height:
        raise RuntimeError('Requested block unavailable')
    tx = {'from': account, 'to': account, 'value': '0x1'}
    queries = [
        ('eth_getBalance', [account, tag]),
        ('eth_getTransactionCount', [account, tag]),
        ('eth_getCode', [account, tag]),
        ('eth_getStorageAt', [account, '0x0', tag]),
        ('eth_call', [tx, tag]),
        ('eth_estimateGas', [tx, tag]),
    ]
    first = [rpc.call(method, params) for method, params in queries]
    deadline = time.monotonic() + 120
    while int(rpc.call('eth_blockNumber', []), 16) <= head:
        if time.monotonic() >= deadline:
            raise RuntimeError('No head advancement within 120 seconds')
        time.sleep(2)
    second = [rpc.call(method, params) for method, params in queries]
    again = rpc.call('eth_getBlockByNumber', [tag, False])
    if first != second or again['hash'] != block['hash']:
        raise RuntimeError('Fixed-height response changed after advancement')
    return {'ok': True, 'chain_id': 4735490, 'height': height, 'block_hash': block['hash'], 'endpoint': url,
            'checks': [{'method': q[0], 'result': value} for q, value in zip(queries, first)],
            'no_transaction_sent': True, 'metamask_certified': False}

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--url', default=DEFAULT_URL)
    p.add_argument('--account', default=DEFAULT_ACCOUNT)
    p.add_argument('--out', default='GATE-historical-api.json')
    args = p.parse_args()
    result = verify(args.url, args.account)
    Path(args.out).write_text(json.dumps(result, indent=2) + '\n')
    print('HVM_HISTORICAL_API_FIXED_HEIGHT_OK')
    print('NO_TRANSACTION_SENT')
    print('PROOF=' + args.out)

if __name__ == '__main__':
    main()
