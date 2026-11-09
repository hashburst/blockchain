#!/usr/bin/env python3
from pathlib import Path

root=Path(__file__).resolve().parent
gate=(root/"run-runtime-provision.sh").read_text()
workflow=(root.parents[2]/".github/workflows/hvm-mainnet-controlled.yml").read_text()

required=[
 "HVM_R7B_MUTATION_BARRIER_4OF4_OK",
 "HVM_R7B_STAGE_4OF4_OK",
 "HVM_R7B_PROVISION_CHECK_4OF4_OK",
 "HVM_R7B_GRACEFUL_RESTART_4OF4_OK",
 "HVM_R7B_CRASH_RECOVERY_4OF4_OK",
 "HVM_R7B_CONVERGENCE_4OF4_OK",
 "RUNTIME_STOPPED_AFTER_ACCEPTANCE",
 "NO_FINAL_PRODUCTION_ACTIVATION",
 "FOUNDER_PRIVATE_KEY_NOT_USED",
 "HVM_R7B_MAINNET_RUNTIME_CRASH_RECOVERY_ACCEPTANCE_OK",
]
for marker in required:
    assert marker in gate, marker
assert "founder-transfers.json" not in gate
assert "wallet.key" not in gate
assert "operator.key" not in gate
assert "reward.key" not in gate
assert "identity/public.json" in gate
assert "R5_IDENTITY_RUN_ID=42503488654" in gate
for digest in (
    "0abe3264bffb9b73c4e4ab8207eeb7056637f9958e2e5b47c45cdc5219bd0267",
    "d3a4a4bb6fe8d03a23be8b2c04baded5802ef7faab3cb3fa9f974576677bfb69",
    "f165f381f1035dc4811866e5d73be06bcbc9eada2dfa1b2f5d37922d99a54d60",
    "ef89a7254902acefcef69326e2979259de9fdb4282fc0cd22a60849f5427a67c",
):
    assert digest in gate
assert "HVM_R7B_ACCEPTED_R5_PUBLIC_IDENTITIES_VERIFIED_4OF4_OK" in gate
assert gate.count("<<'REMOTE'") >= 3
assert "curl -fsS http://127.0.0.1:18019/health" in gate
assert "curl --fail --silent --show-error --max-time 5 --noproxy '*'" not in gate
assert "HVM_R7B_MUTATION_BARRIER_4OF4_OK" in gate
assert 'test "\\$(sha256sum' not in gate
assert 'test "\\$(systemctl is-active' not in gate
assert "runtime-provision" in workflow
assert "bash -n hvm-network/deploy/mainnet/run-runtime-provision.sh" in workflow
assert 'deploy|acceptance)' in workflow
assert 'Requested mutation phase remains fail-closed' in workflow
print("HVM_R7B_STATIC_BOUNDARY_REGRESSION_OK")

# Canonical-script regressions: prevent duplicated bodies and truncated preflight.
assert gate.count("HVM_R7B_MUTATION_BARRIER_4OF4_OK") == 1
assert gate.count("# Stage the exact accepted generation") == 1
assert gate.count("HVM_R7B_MAINNET_RUNTIME_CRASH_RECOVERY_ACCEPTANCE_OK") == 1
assert "grep -Eq '(:18019|:31317)$'" in gate
assert "HVM_R7B_FAIL_CLOSED_RUNTIME_STOPPED_4OF4" in gate
assert "trap cleanup_runtime ERR INT TERM" not in gate

# Run-41 resume/fail-closed regressions.
assert 'fleet must be uniformly clean or uniformly exact R7B-provisioned' in gate
assert 'HVM_R7B_RESUME_EXACT_PROVISIONED_4OF4_OK' in gate
assert 'mixed clean/provisioned fleet state' in gate
assert 'trap cleanup_runtime EXIT INT TERM' in gate
assert 'cleanup_armed=1' in gate
assert 'cleanup_armed=0' in gate
assert 'R7B_CONVERGENCE_SAMPLE attempt=' in gate
assert gate.count('HVM_R7B_MUTATION_BARRIER_4OF4_OK') == 1

# Consolidated resume regressions: recognize only the exact at-rest binary left by accepted predecessor run #49.
# The current rebuild digest is intentionally not the classifier contract for an already-provisioned fleet.
assert '"$ip" "$BIN_SHA" "$LEDGER_DAT_SHA" "$LEDGER_IDX_SHA"' not in gate
assert 'd.get("app_ready") is True' not in gate
assert 'ACCEPTED_PROVISIONED_BINARY_SHA="5a0cc087d757b7659698f92459490ae15bdf4471763360d6d2a0098545268e94"' in gate
assert '"$ip" "$ACCEPTED_PROVISIONED_BINARY_SHA" "$LEDGER_DAT_SHA" "$LEDGER_IDX_SHA"' in gate
assert 'reason=installed_binary_sha' in gate


# Post-#95 provisioned-fleet upgrade contract: the accepted predecessor digest is
# classifier-only. A resumed fleet must atomically install the current reviewed
# binary before any systemctl start, without reprovisioning protected state.
assert 'HVM_R7B_PROVISIONED_BINARY_UPGRADE_4OF4_OK' in gate
assert 'R7B_HOST_BINARY_UPGRADE_OK' in gate
assert '.hashburst-mainnet.r7b-candidate' in gate
assert 'test "$(sha256sum "$opt/hashburst-mainnet" | awk \'{print $1}\')" = "$accepted_sha"' in gate
assert 'test "$(sha256sum "$opt/.hashburst-mainnet.r7b-candidate" | awk \'{print $1}\')" = "$binary_sha"' in gate
assert 'mv -f "$opt/.hashburst-mainnet.r7b-candidate" "$opt/hashburst-mainnet"' in gate
upgrade_pos=gate.index("HVM_R7B_PROVISIONED_BINARY_UPGRADE_4OF4_OK")
start_pos=gate.index("# Controlled temporary runtime start.")
assert upgrade_pos < start_pos

# Height-9 APoW liveness contract: the frozen mainnet protocol activates APoW
# at height 9, so runtime acceptance must admit bounded work before expecting
# finality. The miner identity is ephemeral, miner-only and removed on both
# success and fail-closed cleanup.
assert 'TestMainnetR6Height9RoundAbove64RequiresAPoWAndFinalizesWithWork' in gate
assert 'go build -trimpath -o "$APOW_MINER" ./cmd/hvm-apow-miner' in gate
assert 'HVM_R7B_APOW_ACCEPTANCE_SOURCE_READY' in gate
assert 'HVM_R7B_APOW_ACCEPTANCE_SOURCE_REMOVED_OK' in gate
assert '--chain-id 4735489' in gate
assert '--endpoint http://127.0.0.1:18019/apow' in gate
assert '--generate-key --key-file' in gate
assert 'hashburst-r7b-apow.key' in gate
assert 'systemctl stop \'hashburst-r7b-apow-acceptance.service\'' in gate
assert "rm -f '$OPT/hvm-apow-miner-r7b' /run/hashburst-r7b-apow.key" in gate
assert gate.index("HVM_R7B_APOW_ACCEPTANCE_SOURCE_READY") < gate.index("# Graceful restart each validator")
