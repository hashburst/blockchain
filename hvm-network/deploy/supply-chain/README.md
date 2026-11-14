# HashBurst G5 supply-chain gate

This directory defines the CI-only G5 release evidence pipeline. It does not deploy, restart, provision, or contact fleet hosts.

The pipeline builds the three artifacts needed to retire the current provenance debt: `hashburst-mainnet` (mainnet ingress runtime), `hashburst-testnet` (testnet ingress runtime), and `hvm-mainnet-evm-gateway` (mainnet EVM gateway).

Two independent builds from the same source commit must be byte-identical before metadata is accepted. The bundle contains `SOURCE_COMMIT`, `SHA256SUMS`, CycloneDX `SBOM.cdx.json`, `MANIFEST.json`, and `PROVENANCE.json`.

CI signing uses a fresh Ed25519 test key generated inside the workflow. This demonstrates signing, verification, tamper rejection, and unsigned-package rejection. It is not a production release identity and must not be represented as one. Founder, validator consensus, and P2P keys are never used by this pipeline.

Production deployment remains a separate controlled gate after review, SHA-locked merge, and selection of the production supply-chain signing identity.

## Production provenance identity

Metadata generation requires the builder identity and build process to be
declared explicitly through `G5_BUILDER` and `G5_BUILD_PROCESS`.

CI uses:

    G5_BUILDER=github-actions
    G5_BUILD_PROCESS=hvm-g5-supply-chain.yml

A production build performed on the Founder Control Runner uses:

    G5_BUILDER=founder-control-runner
    G5_BUILD_PROCESS=g5-production-release-v1

These fields describe the actual build environment/process. They must never
be copied from CI metadata when the production artifacts were built elsewhere.

The production release signing key remains separate from the builder identity.
The private release key must remain root-only on the Founder Control Runner and
must never be committed, uploaded as an artifact, or distributed to fleet hosts.
