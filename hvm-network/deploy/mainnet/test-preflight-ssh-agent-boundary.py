#!/usr/bin/env python3
"""Fail closed if mainnet preflight regresses to direct private-key access."""
from pathlib import Path

src = (Path(__file__).with_name("preflight.py")).read_text(encoding="utf-8")

assert "SSH_AUTH_SOCK='/run/hashburst-control/ssh-agent.sock'" in src
assert "'-i'" not in src
assert "fleet_ed25519" not in src
assert "IdentitiesOnly=yes" not in src
assert "'SSH_AUTH_SOCK':SSH_AUTH_SOCK" in src

print("HVM_MAINNET_PREFLIGHT_SSH_AGENT_BOUNDARY_OK")
print("PRIVATE_KEY_DIRECT_READ=no")
