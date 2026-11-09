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
assert 'd.get("app_ready") is True' in gate
assert "curl --fail --silent --show-error --max-time 5 --noproxy '*'" in gate
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
