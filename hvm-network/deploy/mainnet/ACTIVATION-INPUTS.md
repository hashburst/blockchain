# Production activation inputs

The economic genesis implementation is merged. The managed legacy archive is
already accepted; no additional archive rollout is required.

A production validator manifest cannot be derived from testnet peer IDs, host IP
addresses or approval of a software change. The following public inputs are
required before preparing the validator checkpoint:

1. Selected mainnet validator and observer hosts, with an explicit role per host.
2. Fresh mainnet public registrations: node ID, P2P peer ID, consensus public key,
   operator/reward addresses, real TEP public identity and signatures binding
   those values to chain 4735489. Private keys remain on each owning host.
3. Approved validator bonds and voting weights, funding transactions and fees,
   activation heights, consensus timeouts, PoH ticks, gas limit/base fee and APoW
   difficulty adjustment parameters. Testnet fixture values are not defaults.
4. Explicit reward schedule after the initial 50 HBT subsidy and faucet budget,
   if enabled. The faucet draws from the founder allocation, not new issuance.

The builder must conserve the approved initial allocation while funding bonds
and fees. Synthetic testnet registration blocks generate subsidies and must not
be substituted for this production preparation.

Final acceptance binds the complete protocol, public validator set, genesis and
checkpoint hashes, economic reconciliation, consumed-source commitment and
runtime restart/finality results to the exact release source commit. The existing
freeze report supplies scoped legacy evidence; it is not proof that a new
mainnet validator is configured or running.

No production identity or missing policy value has been generated from a
placeholder. Publication of the final release and live website activation claims
remains pending these inputs and runtime acceptance.
