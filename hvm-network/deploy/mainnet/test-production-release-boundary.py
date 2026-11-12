#!/usr/bin/env python3
from pathlib import Path
g=Path(__file__).with_name("run-production-release.sh").read_text()
r=Path(__file__).with_name("mainnet-evm-routes.conf").read_text()
m=(Path(__file__).resolve().parents[2]/"cmd/hvm-mainnet-evm-gateway/main.go").read_text()
assert 'evmgateway.New(\"http://127.0.0.1:18019/evm\", \"ws://127.0.0.1:18019/evm/ws\"' in m
assert "127.0.0.1:18020" not in m
assert "R8A_RUN=42761659413" in g
assert "R8A_ARTIFACT_DIGEST=2b80e4ed1b24656c9d29ebc62e7b834ee18d39c15eb4d55183793b713abc3b85" in g
assert "HVM_R8B_VALIDATOR_AT_REST_AND_TESTNET_OBSERVER_PRESERVED_OK" in g
assert "sha256sum /opt/hashburst-hvm-mainnet/hashburst-mainnet | cut -d ' ' -f1" in g
assert "awk '{print \\\\$1}'" not in g
assert "role=\"observer\"" in g and "consensus_key_file=\"\"" in g
assert 'data_dir="/var/lib/hashburst-hvm-mainnet-ingress"' in g
assert 'p2p_key_file="/etc/hashburst-hvm-mainnet-ingress/p2p.key"' in g
assert 'rpc_listen="127.0.0.1:18019"' in g
assert "p2p_port=31317" in g
assert "/var/lib/hashburst-hvm-mainnet-observer" not in g
assert "/etc/hashburst-hvm-mainnet-observer" not in g
assert 'rpc_listen="127.0.0.1:18020"' not in g
assert "p2p_port=31318" not in g
assert "HVM_R8B_ROLLBACK_ARMED_BEFORE_OBSERVER_MUTATION_OK" in g
assert g.index("HVM_R8B_ROLLBACK_ARMED_BEFORE_OBSERVER_MUTATION_OK") < g.index("getent passwd '$OBS_USER'")
assert "test -e '$OBS_STATE/runtime.pin'" in g
assert "test -f '$OBS_STATE/runtime.pin'" in g
assert "test ! -L '$OBS_STATE/runtime.pin'" in g
assert "--config '$OBS_ETC/node.json' --check" in g
assert "HVM_R8B_MAINNET_OBSERVER_EXISTING_PIN_REVALIDATED_OK" in g
assert "HVM_R8B_MAINNET_OBSERVER_FRESH_PROVISION_OK" in g
assert "HVM_R8B_OBSERVER_BACKED_PUBLIC_RPC_INGRESS_ACCEPTANCE_OK" in g
assert "http://127.0.0.1:18021/rpc" in g
assert "HVM_R8B_MAINNET_EVM_GATEWAY_LOCAL_OK" in g
assert "--resolve blockchainapi.one:443:127.0.0.1" in g
assert "HVM_R8B_NGINX_LOCAL_TLS_SNI_ROUTE_OK" in g
assert "expected one blockchainapi.one HTTPS server" in g
assert "mainnet include exists outside canonical HTTPS server" in g
assert "testnet route anchor missing or ambiguous in HTTPS server" in g
assert "for n in \\$(seq 1 15); do" in g
assert "/root/hashburst-hvm-mainnet-nginx-diagnostics.sh || true" in g
d=Path(__file__).with_name("diagnose-mainnet-nginx.sh").read_text()
assert "nginx -T" in d
assert "HTTPS_BLOCKS=" in d
assert "http://127.0.0.1:18021/rpc" in d
assert "--resolve blockchainapi.one:443:127.0.0.1" in d
assert "x-hvm-evm:" in d
assert "E=/etc/nginx/sites-enabled/blockchainapi.one.conf" in g
assert 'test -L \\"\\$E\\"' in g
assert 'readlink -f \\"\\$E\\"' in g and 'readlink -f \\"\\$V\\"' in g
assert "nginx -T 2>&1 | grep -Fq 'include /etc/nginx/snippets/hvm-mainnet-evm.conf;'" in g
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
