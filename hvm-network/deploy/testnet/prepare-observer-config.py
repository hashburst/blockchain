#!/usr/bin/env python3
"""Prepare an unsigned observer config from an approved public testnet config.
The peer ID must come from a fresh P2P key generated on the ingress host.
This command writes a NEW file only; it does not provision or start a service.
"""
import argparse,json
def main():
 p=argparse.ArgumentParser();p.add_argument('--template',required=True);p.add_argument('--peer-id',required=True);p.add_argument('--out',required=True)
 a=p.parse_args()
 with open(a.template) as f:c=json.load(f)
 if c['network']!='testnet' or c['protocol']['chain_id']!=4735490:raise ValueError('wrong testnet')
 if not a.peer_id or a.peer_id==c['peer_id'] or any(b.endswith('/p2p/'+a.peer_id) for b in c['bootnodes']):raise ValueError('observer needs its own P2P identity')
 c.update(node_id='hvm-testnet-ingress',role='observer',peer_id=a.peer_id,validator_id='',consensus_key_file='',data_dir='/var/lib/hashburst-hvm-testnet-ingress',p2p_key_file='/etc/hashburst-hvm-testnet-ingress/p2p.key',rpc_listen='127.0.0.1:18009',p2p_listen_ip='0.0.0.0',p2p_port=31307)
 with open(a.out,'x') as f:json.dump(c,f,indent=2)
 print('OBSERVER_CONFIG_PREPARED_NO_SERVICE_STARTED')
if __name__=='__main__':main()
