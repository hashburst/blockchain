#!/usr/bin/env python3
import json
from pathlib import Path

fixture = Path(__file__).with_name("fixtures") / "r6-validator-checkpoint-shape.json"
d = json.loads(fixture.read_text())
assert d["schema"] == "hashburst-mainnet-validator-checkpoint-v1"
assert d["activation_allowed"] is False
assert d["runtime_acceptance_required"] is True
assert d["chain_id"] == 4735489
assert d["bootstrap_issuance_units"] == 0
vs = d["validator_set"]
assert isinstance(vs, dict)
assert vs["height"] == d["checkpoint_height"] + 1
assert len(vs["validators"]) == 4
assert vs["power"] == [1, 1, 1, 1]
assert all(v["status"] == "ACTIVE" for v in vs["validators"])
print("HVM_R7_R6_CHECKPOINT_FIXTURE_REGRESSION_OK")
