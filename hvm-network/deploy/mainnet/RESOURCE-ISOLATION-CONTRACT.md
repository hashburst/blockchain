# HVM Resource Isolation Acceptance Contract

This repository owns HVM/HBT protocol behavior, including the HBT native coin, mainnet/testnet, validator consensus, APoW validity/difficulty/reward rules, EVM execution and HBT-compatible smart-contract standards (including HBT-20 and HBT-721 families).

Mining optimization, segmentation, telemetry and AI are external node capabilities. They are implemented in `hashburst/mining-accelerator`, `hashburst/AI` and `hashburst/mining_segmentation`, coordinated by `hashburst/node` and composed by `hashburst/node-installer`.

## Acceptance boundary

HVM may test or gate the following integration properties:

- validator/HVM service remains healthy while an external mining workload is active;
- consensus/finality latency remains within the approved HVM envelope;
- mining resource actuation cannot alter HVM consensus parameters, APoW validity/reward rules, validator identities or HBT economics;
- failure of an external optimizer fails closed with respect to protected HVM capacity;
- integration evidence records component/version identifiers without embedding mining AI implementation in this repository.

Historical diagnostics, dry-run, simulation and canary results remain valid evidence of the correction path. Their successful gates are not invalidated by moving mining-specific implementation to its owning repositories.

Future HVM mainnet progression after this boundary is validator identities -> checkpoint -> deploy -> acceptance.
