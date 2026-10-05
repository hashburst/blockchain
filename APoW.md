# HashBurst APoW

The current HVM APoW protocol is specified in
[hvm-network/docs/APOW-BFT-V1.md](hvm-network/docs/APOW-BFT-V1.md).

A valid work submission is not a finalized block or an earned reward. The
protocol binds work to its signer and beneficiary, and the subsidy is applied
once when the block satisfies the configured rules and BFT finality.

The earlier adaptive-mining experiments in this repository are not the HVM
consensus specification. Miner scheduling and external optimization models must
not change network difficulty, supply, signatures or validation rules outside
an explicitly approved protocol configuration. Energy reduction requires
measurements; it cannot be inferred from an algorithm name or CPU utilization.
