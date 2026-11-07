#!/usr/bin/env python3
"""Derive a mainnet parameter proposal from live read-only measurements. Never activates a network."""
import argparse,json,math,statistics
from pathlib import Path

def proposal(doc):
 rows=doc.get('nodes',[])
 if len(rows)!=5 or not all(r.get('preflight_ok') is True for r in rows):raise ValueError('five successful live preflights required')
 vals=[float(r['apow_hashes_per_second']) for r in rows[:4]]
 if any(not math.isfinite(x) or x<=0 for x in vals):raise ValueError('invalid APoW measurement')
 # Keep expected reference work around <=1s on slowest validator/miner, bounded by tested range.
 bits=max(8,min(20,math.floor(math.log2(min(vals)))))
 # EVM recommendation is bounded by measured successful transfer throughput and latency.
 gas=[int(r['evm_safe_gas_limit']) for r in rows]
 if any(x<21000 or x>30000000 for x in gas):raise ValueError('invalid EVM measurement')
 gas_limit=min(gas)
 return {'schema':1,'chain_id':4735489,'status':'MEASURED_PROPOSAL_NOT_ACTIVATED',
  'apow':{'initial_bits':bits,'min_bits':max(8,bits-4),'max_bits':min(24,bits+2),'window':32,'target_seconds':5,
          'slowest_hashes_per_second':min(vals),'median_hashes_per_second':statistics.median(vals)},
  'evm':{'gas_limit':gas_limit,'base_fee_wei':1},
  'requirements':['independent review','protocol/economics freeze','founder funding signature','4+1 acceptance']}
def main():
 p=argparse.ArgumentParser();p.add_argument('measurements');p.add_argument('--out',required=True);a=p.parse_args()
 out=proposal(json.loads(Path(a.measurements).read_text()));Path(a.out).write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out,indent=2));print('MAINNET_PARAMETERS_PROPOSED_NOT_FROZEN')
if __name__=='__main__':main()
