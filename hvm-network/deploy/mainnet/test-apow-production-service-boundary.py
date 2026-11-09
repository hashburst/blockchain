from pathlib import Path

root = Path(__file__).resolve().parents[2]
gate = (root / "deploy/mainnet/run-apow-production-service.sh").read_text()
workflow = (root.parent / ".github/workflows/hvm-mainnet-controlled.yml").read_text()

assert "CHAIN_ID=4735489" in gate
assert "APOW_HOST=77.90.188.153" in gate
assert "MINER_USER=hashburst-apow-miner" in gate
assert "MINER_SERVICE=hashburst-apow-mainnet.service" in gate
assert "KEYDIR=/var/lib/hashburst-apow-mainnet" in gate
assert "ENDPOINT=http://127.0.0.1:18019/apow" in gate
assert "--chain-id $chain --loop --timeout 120s" in gate
assert "Restart=on-failure" in gate
assert "CPUQuota=25%" in gate
assert "Nice=15" in gate
assert "NoNewPrivileges=true" in gate
assert "ProtectSystem=strict" in gate
assert "RestrictAddressFamilies=AF_INET AF_INET6" in gate
assert "systemd-analyze verify" in gate
assert 'sha=${10}; validator=${11}' in gate
assert "sha=10" not in gate
assert "validator=11" not in gate

# Execute the same 11-argument positional binding shape used by the remote
# heredoc. This catches the exact class of failure that made run #54 resolve
# the validator service as 11.service before any service/key provisioning.
import subprocess
probe = r'''set -euo pipefail
set -- u s b kd k bin unit endpoint 4735489 deadbeef hashburst-hvm-mainnet.service
user=$1; service=$2; base=$3; keydir=$4; key=$5; bin=$6; unit=$7; endpoint=$8; chain=$9; sha=${10}; validator=${11}
test "$chain" = 4735489
test "$sha" = deadbeef
test "$validator" = hashburst-hvm-mainnet.service
printf "%s|%s|%s\\n" "$chain" "$sha" "$validator"
'''
p = subprocess.run(["bash", "-c", probe], check=True, capture_output=True, text=True)
assert p.stdout.strip() == "4735489|deadbeef|hashburst-hvm-mainnet.service"
assert "systemctl start '$MINER_SERVICE'" in gate
assert "kill -KILL" in gate
assert "HVM_R8A_APOW_SERVICE_RESTART_RECOVERY_OK" in gate
assert "HVM_R8A_APOW_CUSTODY_BOUNDARY_OK" in gate
assert "NO_APOW_PROOF_SUBMITTED" in gate
assert "NO_PRODUCTION_ACTIVATION" in gate
assert "FOUNDER_PRIVATE_KEY_NOT_USED" in gate
assert "VALIDATOR_PRIVATE_KEY_NOT_USED" in gate
assert "consensus.key" not in gate
assert "founder.key" not in gate.lower()
assert "founder-signed" not in gate.lower()
assert "systemctl start '$VALIDATOR_SERVICE'" not in gate
assert "systemctl restart '$VALIDATOR_SERVICE'" not in gate
assert "systemctl enable '$VALIDATOR_SERVICE'" not in gate
assert "protocol.freeze.json" in gate
assert "initial_bits\":20" in gate
assert "min_bits\":16" in gate
assert "max_bits\":32" in gate
assert "target_seconds\":60" in gate
assert "apow-production-service" in workflow
assert "run-apow-production-service.sh" in workflow
assert "test-apow-production-service-boundary.py" in workflow
assert "Requested mutation phase remains fail-closed" in workflow
assert "HVM_DEPLOY_ACCEPTANCE_PHASES_FAIL_CLOSED_OK" in workflow
print("HVM_R8A_STATIC_BOUNDARY_REGRESSION_OK")
