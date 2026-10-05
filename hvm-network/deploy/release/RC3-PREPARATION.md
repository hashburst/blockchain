# HVM Network v0.7.0-rc.3 - draft preparation

This draft follows PR #30, merged as 491f0cee03c3acc49b3439008340ca6ee6d4efbd.
It includes the immutable legacy terminal archive, reviewed HA transition
acceptance, pinned migration candidate and offline double-import rejection model.
It does not activate mainnet, import assets or update any VPS or public website.

Legacy 1337: terminal index 10, 450 HBT, zero terminal issuance.
Testnet 4735490: independent state and identities, on-demand .dat/.idx ledger.
Mainnet 4735489: not activated. The consumed import and balances must still be
committed together by the actual bootstrap/replay implementation.

Freeze evidence SHA256:
`d0f2246fc26e9e72c6117ac0d6f6805c7427a8cc1b106613c822a1cbb7e0a6f6`.
Scope: five managed legacy services at the acceptance observation times.

## Final publication prerequisites

1. Implement and test consensus-bound balances plus consumed-source commitment,
   duplicate import rejection, crash recovery and supply reconciliation.
2. Complete protocol and validator manifest with independent mainnet identities,
   initial checkpoint, source commitments and explicit economic units.
3. Accept mainnet runtime, restart, finality and wallet/reward behavior against
   the same reviewed source and manifest. No legacy rollout repetition is needed.
4. Build and verify the final source on supported platforms; bind checksums,
   source commit and acceptance reports to the release. Windows full-node support
   is not included in the current platform claim.
5. Publish the final release and reviewed website content only after these gates.
   Never promote this draft by merely changing its title or activation flag.

The draft packaging workflow creates no stable tag publication and preserves an
existing draft unchanged on reruns. Artifacts retain their own SOURCE_COMMIT.
