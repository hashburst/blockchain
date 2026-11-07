#!/usr/bin/env python3
import hashlib,json,pathlib
root=pathlib.Path(__file__).resolve().parent
pp=root/"protocol.freeze.json"; ep=root/"economics.freeze.json"
p=json.loads(pp.read_text()); e=json.loads(ep.read_text())
def req(ok,msg):
    if not ok: raise SystemExit("FREEZE_INVALID: "+msg)
req(p["chain_id"]==4735489,"chain id")
req(p["activation_height"]==1,"protocol activation")
req(3<=p["mainnet_bootstrap_end"]<=4096,"bootstrap bound")
req(p["consensus_activation_height"]==p["mainnet_bootstrap_end"]+1,"consensus boundary")
req(p["evm"]["activation_height"]>=p["consensus_activation_height"],"EVM activation")
req(p["apow"]["activation_height"]>=p["consensus_activation_height"],"APoW activation")
req(p["poh_ticks_per_block"]==400000,"production PoH ticks")
req(p["genesis_import"]["units"]==45000000000,"legacy units")
req(p["genesis_import"]["founder_units"]==100000000000000000,"founder units")
req(p["validator"]["min_bond_units"]==100000000000,"validator bond")
req(p["apow"]["min_bits"]<=p["apow"]["initial_bits"]<=p["apow"]["max_bits"],"APoW bits")
req(p["apow"]["target_seconds"]==60 and p["apow"]["window"]==60,"APoW target/window")
req(e["activation_allowed"] is False,"economics self-activation")
req(e["initial_supply"]["total_initial_units"]=="100000045000000000","initial supply")
req(e["apow_emission"]["reward_units"]=="5000000000","APoW reward")
req(e["faucet"]["source"].startswith("subset of founder allocation"),"faucet source")
digest=hashlib.sha256(json.dumps(p,sort_keys=True,separators=(",",":")).encode()).hexdigest()
req(e["protocol_freeze_sha256"]==digest,"protocol digest linkage")
print("HVM_MAINNET_PROTOCOL_ECONOMICS_FREEZE_VALID")
print("protocol_sha256="+digest)
print("economics_sha256="+hashlib.sha256(ep.read_bytes()).hexdigest())
