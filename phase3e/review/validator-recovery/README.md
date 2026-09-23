# Persistent validator recovery — targeted verification

Local tests on 2026-09-23, Go 1.25.7; no production or VPS deployment.
Phase3E RC3 gates are not reopened by this follow-up.

## Reproduce from phase3e/

```bash
export GOTOOLCHAIN=local
go test -race ./blockchain ./consensus ./internal/testnet \
  -run 'TestPersistentRecovery|TestJournalPersistence|TestRestartGuard|TestStatePin|TestRejectMissing|TestConfiguration|TestPersistentObserver' \
  -count=1 -v
HB_TESTNET_INTEGRATION=1 go test ./internal/testnet \
  -run '^TestFourPersistentValidatorProcesses$' -count=1 -v
go build -trimpath -o /tmp/hashburst-testnet ./cmd/hashburst-testnet
```

The execution environment used GOFLAGS=-buildvcs=false because Git metadata
stamping is unavailable there. This does not change runtime code. Unit fixtures
and process fixtures create isolated temporary chains/keys only. The runtime
entry point never generates a genesis.

## Tests

- Proposal, nil prevote, prevote and certified precommit cold restart.
- Crash between snapshot reservation and journal signature; repeated restarts.
- Recovery of locked block and prevote QC; rejection of an otherwise valid but
  conflicting later-round proposal without an unlocking certificate.
- All four validators reopen their on-disk chains/journals/snapshots at an
  unfinalized locked height and finalize the same value through a fresh round.
- Missing, corrupt, older, wrong-identity and invalid-QC recovery state rejected.
- Journal ahead of snapshot and exhausted round budget rejected without signing.
- Failed snapshot write and uncertain journal append latch failure until restart.
- Existing runtime identity pin, duplicate process lock, corruption and observer
  lifecycle checks remain active.
- Four real validator processes finalize; observer restart preserves its journal;
  re-enabled validator survives SIGKILL/restart on the same disk and emits new
  precommits. The journal prefix is unchanged and all nodes advance finality.

`race.txt` and `process.txt` contain the corresponding execution evidence.
`SHA256SUMS` binds the changed implementation/test/doc files in this follow-up.

## Boundaries

Persistence is enabled for strict OpenExistingBlockchain runtimes. Existing
Phase3E ephemeral harness behavior and public legacy entry point are not switched.
Atomic recovery snapshots do not repair torn blockchain.dat/blockchain.idx writes.
These remain fail-closed. Old validators with pending signatures but no snapshot
must catch up as observers if a live quorum exists. No automatic journal deletion,
key migration, public service installation or chain reset is included.

This validates the implemented crash scenarios; it is not a proof against arbitrary
storage failure, disk rollback or a malicious operator with signing keys. Preserve
chain, snapshot and journals as one coherent state and do not clone active keys.
Multi-host deployment and independent review precede public activation.

## Chain ID decision retained

- Legacy/pre-HVM: 1337 remains unchanged.
- HVM Testnet: distinct ID to select and verify before public activation.
- HVM Mainnet: another distinct ID to select and verify before public activation.

No numeric public HVM IDs are assigned by this patch. Config pinning means these
must be set consistently before provisioning their respective public networks;
changing an existing chain ID in place is not a supported migration.
