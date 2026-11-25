#!/usr/bin/env python3
from pathlib import Path

root = Path(__file__).resolve().parents[3]
workflow = (root / ".github/workflows/hvm-mainnet-controlled.yml").read_text()
dispatcher = Path(__file__).with_name("production-release-recovery-execute.sh")

assert "production-release-recovery" in workflow
assert "production-release|production-release-recovery|deploy|acceptance" in workflow
assert "github.event.inputs.phase == 'production-release-recovery'" in workflow
assert "production-release-recovery)" in workflow
assert "sudo -n /usr/local/libexec/hashburst-deployment-agent/production-release-recovery-execute" in workflow

recovery_case = workflow.split("production-release-recovery)", 1)[1].split(";;", 1)[0]
assert "HASHBURST_RELEASE_MODE" not in recovery_case
assert "HASHBURST_RECOVERY_TARGET" not in recovery_case
assert "production-release-execute" not in recovery_case
assert "production-release-recovery-execute" in recovery_case

assert dispatcher.is_file()
d = dispatcher.read_text()
assert "set -euo pipefail" in d
assert "HASHBURST_RELEASE_MODE=rollback-only" in d
assert "HASHBURST_RECOVERY_TARGET=77.90.188.153" in d
assert "export HASHBURST_RELEASE_MODE HASHBURST_RECOVERY_TARGET" in d
assert 'exec "$TRUSTED_ORCHESTRATOR"' in d
assert "run-production-release.sh" in d
assert '"$@"' not in d
for forbidden in ("77.90.188.154", "77.90.188.155", "77.90.188.157"):
    assert forbidden not in d

print("HVM_A3_R5A_RECOVERY_DISPATCH_BOUNDARY_REGRESSION_OK")
