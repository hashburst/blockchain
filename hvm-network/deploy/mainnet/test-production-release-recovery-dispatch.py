#!/usr/bin/env python3
from pathlib import Path

root = Path(__file__).resolve().parents[3]
workflow = (root / ".github/workflows/hvm-mainnet-controlled.yml").read_text()

assert "production-release-recovery" in workflow
assert "production-release|production-release-recovery|deploy|acceptance" in workflow
assert "github.event.inputs.phase == 'production-release-recovery'" in workflow
assert "production-release-recovery)" in workflow

recovery_case = workflow.split("            production-release-recovery)\n", 1)[1].split("              ;;", 1)[0]
expected = "sudo -n /usr/local/libexec/hashburst-deployment-agent/production-release-recovery-execute"
assert expected in recovery_case
assert "HASHBURST_RELEASE_MODE" not in recovery_case
assert "HASHBURST_RECOVERY_TARGET" not in recovery_case
assert "production-release-execute" not in recovery_case
assert "$" not in recovery_case.split(expected, 1)[1]
assert "77.90.188.153" not in workflow

print("HVM_A3_R5A_RECOVERY_DISPATCH_BOUNDARY_REGRESSION_OK")
