from pathlib import Path

root = Path(__file__).resolve().parents[3]
gate = (root / "deploy/mainnet/run-activation-readiness.sh").read_text()
workflow = (root.parent / ".github/workflows/hvm-mainnet-controlled.yml").read_text()

assert "R7B_RUN_ID=42616952745" in gate
assert "R7B_RUN_NUMBER=51" in gate
assert "R7B_SOURCE_SHA=c07ffa33740fb369dafccfce680e86b8554bb1b4" in gate
assert "R7B_ARTIFACT_ID=13673827024" in gate
assert "R7B_ARTIFACT_SHA256=a97f82f0359ce805246c94cad27e5d136965eeabe765f07d94ef796fc738a543" in gate
assert "R7B_BINARY_SHA256=cf111db0d141e76787b76cc319f97fa6a4608de2868d4ba0d94381ba853fb2b8" in gate
assert "R7B_ACCEPTED_FINALIZED_HEIGHT=29" in gate
assert "R7B_FINALIZED_HASH=dcaf4cf790cfca78bc9ac2cefa0b93e6e8264685041a0bb6a8df7e7f4ca6b844" in gate
assert '"production_activation_authorized": false' in gate
assert "HVM_R7C_CANONICAL_R7B_RUN_51_BOUND_OK" in gate
assert "HVM_R7C_VALIDATORS_STOPPED_DISABLED_4OF4_OK" in gate
assert "NO_SERVICE_STARTED" in gate
assert "NO_PRODUCTION_ACTIVATION" in gate
assert "FOUNDER_PRIVATE_KEY_NOT_USED" in gate
assert "systemctl start" not in gate
assert "systemctl enable" not in gate
assert "systemctl restart" not in gate
assert "rm -f '$STATE" not in gate
assert "truncate" not in gate
assert "activation-readiness" in workflow
assert "run-activation-readiness.sh" in workflow
assert 'deploy|acceptance)' in workflow
assert "Requested mutation phase remains fail-closed" in workflow
print("HVM_R7C_STATIC_BOUNDARY_REGRESSION_OK")
