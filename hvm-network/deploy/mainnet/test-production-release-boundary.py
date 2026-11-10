#!/usr/bin/env python3
from pathlib import Path
g=Path(__file__).with_name("run-production-release.sh").read_text()
r=Path(__file__).with_name("mainnet-evm-routes.conf").read_text()
assert "R8A_RUN=42761659413" in g
assert "R8A_ARTIFACT_DIGEST=2b80e4ed1b24656c9d29ebc62e7b834ee18d39c15eb4d55183793b713abc3b85" in g
assert "HVM_R8B_VALIDATOR_AT_REST_AND_TESTNET_OBSERVER_PRESERVED_OK" in g
assert "sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | cut -d ' ' -f1" in g
assert "awk '{print \\\\$1}'" not in g
assert "role=\"observer\"" in g and "consensus_key_file=\"\"" in g
assert "rpc_listen=\"127.0.0.1:18020\"" in g
assert "HVM_R8B_OBSERVER_BACKED_PUBLIC_RPC_INGRESS_ACCEPTANCE_OK" in g
assert "admin_peers" in g and "-32601" in g
assert "HVM_R8C_R8D_PUBLIC_CANARY_EVIDENCE_COLLECTED_OK" in g
assert "/var/lib/hashburst-apow-mainnet/miner.key" in g
assert "HVM_R8E_CANONICAL_PRODUCTION_RELEASE_MANIFEST_OK" in g
assert "HVM_R9_FINAL_PRODUCTION_ACTIVATION_DEPLOY_ACCEPTANCE_OK" in g
assert "HVM_R9_FAIL_CLOSED_PRODUCTION_SERVICES_STOPPED" in g
assert "FOUNDER_PRIVATE_KEY_NOT_USED" in g
assert "VALIDATOR_PRIVATE_KEYS_NOT_EXPORTED" in g
assert "/api/hashburst/hvm/mainnet/evm" in r
assert "127.0.0.1:18021" in r
assert "proxy_set_header Upgrade" in r
for forbidden in ("founder.key","consensus.key'","/etc/hashburst-hvm-mainnet/consensus.key"):
    assert forbidden not in g
print("HVM_R8B_R9_STATIC_BOUNDARY_REGRESSION_OK")
