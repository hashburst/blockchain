#!/usr/bin/env python3
from pathlib import Path
w=(Path(__file__).parents[3]/'.github/workflows/hvm-mainnet-controlled.yml').read_text()
d=(Path(__file__).parent/'CONTROL-RUNNER.md').read_text()
assert 'runs-on: [self-hosted, linux, x64, hashburst-control, founder-180]' in w
assert 'environment: hvm-mainnet-production' in w
assert 'StrictHostKeyChecking=yes' in w and 'BatchMode=yes' in w
assert 'protocol.freeze.json' in w and 'economics.freeze.json' in w
assert '77.90.188.180' in w
assert 'GitHub Actions receives no fleet private key' in d
assert 'founder HBT wallet key never exists on this coordinator' in d
print('HVM_MAINNET_CONTROL_RUNNER_POLICY_OK')
