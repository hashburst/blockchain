#!/usr/bin/env python3
"""Regression contract for read-only production A2 preflight."""
from pathlib import Path
root=Path(__file__).parent
coord=(root/'preflight.py').read_text()
node=(root/'preflight-node.py').read_text()
assert "64.31.4.9" not in coord
assert "len(rows)==4" in coord
assert "HVM_MAINNET_PRODUCTION_PREFLIGHT_4OF4_OK" in coord
assert "MAINNET_PREFLIGHT_NO_MUTATION" in coord
assert "hvm-mainnet-production" in node
assert "systemctl','is-active" in node
assert "systemctl','is-enabled" in node
assert "http://127.0.0.1:18019/health" in node
assert "86a9c1e1ed65c2e3afcaf469a3155fad94add9e88a2b90e5bc80e0c2e33b3fb3" in node
assert "candidate mainnet port unavailable" not in node
assert "unexpected mainnet service already active" not in node
assert ".bind(" not in node
print("HVM_MAINNET_PRODUCTION_PREFLIGHT_CONTRACT_OK")
print("FLEET_MUTATION=no")
print("SERVICE_MUTATION=no")
