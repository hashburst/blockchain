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
assert "runtime-provision" in workflow
assert "bash -n hvm-network/deploy/mainnet/run-runtime-provision.sh" in workflow
assert 'deploy|acceptance)' in workflow
assert 'Requested mutation phase remains fail-closed' in workflow
print("HVM_R7B_STATIC_BOUNDARY_REGRESSION_OK")
