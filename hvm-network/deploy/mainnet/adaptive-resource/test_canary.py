#!/usr/bin/env python3
import pathlib
p=pathlib.Path(__file__).with_name("run-canary.sh").read_text()
assert "CANARY=77.90.188.153" in p
assert "CONTROL=77.90.188.154" in p
assert 'root@"$CONTROL" "bash -se"' not in p
assert "CPUQuota=1200%" in p
assert "--pid=\"$pid\"" in p
assert "miner_restart\":False" in p
assert "sample-node.py" in p
print("HASHBURST_ADAPTIVE_RESOURCE_CANARY_SCOPE_TESTS_OK")
