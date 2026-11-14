# HashBurst G5 supply-chain gate

This directory defines the CI-only G5 release evidence pipeline. It does not deploy, restart, provision, or contact fleet hosts.

The pipeline builds the three artifacts needed to retire the current provenance debt: `hashburst-mainnet` (mainnet ingress runtime), `hashburst-testnet` (testnet ingress runtime), and `hvm-mainnet-evm-gateway` (mainnet EVM gateway).

Two independent builds from the same source commit must be byte-identical before metadata is accepted. The bundle contains `SOURCE_COMMIT`, `SHA256SUMS`, CycloneDX `SBOM.cdx.json`, `MANIFEST.json`, and `PROVENANCE.json`.

CI signing uses a fresh Ed25519 test key generated inside the workflow. This demonstrates signing, verification, tamper rejection, and unsigned-package rejection. It is not a production release identity and must not be represented as one. Founder, validator consensus, and P2P keys are never used by this pipeline.

Production deployment remains a separate controlled gate after review, SHA-locked merge, and selection of the production supply-chain signing identity.
