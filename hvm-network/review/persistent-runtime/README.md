# Persistent testnet runtime verification

Historical evidence for commit `b2e3fa29f9bf957fb01a9fec4d0cc08315793a38`; its SHA256SUMS refer to that revision.
The subsequent validator-recovery change has separate evidence in `../validator-recovery/`.

Tests executed locally on 2026-09-23 with Go 1.25.7. Not VPS or public-network results.

- Full suite: PASS (process integration opt-in is skipped in this ordinary run).
- Race: internal/testnet, blockchain, consensus PASS.
- Explicit process integration: four real validator processes and subsequent observer restart/catch-up PASS. Requires progress beyond the height observed after stopping the node. Observer signing journal unchanged.
- Missing/corrupt chain, corrupt index/journal, identity/config pin mismatch and duplicate-process lock rejection covered.
- RPC is loopback only and /control/start returns 404.

No netns impairment test or multi-host rollout of this new entry point has been performed. RC3 netns evidence concerns the previous harness. This change does not claim full cold-validator recovery with pending signing records; startup fails closed in that situation.

See deploy/testnet/README.md for build, reproduction and deployment prerequisites. No production services or chain IDs were modified.
