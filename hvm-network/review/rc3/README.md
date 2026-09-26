# RC3 validation record

The .log files were generated in the review environment on 2026-09-23, before import. vps-acceptance.txt is the separate user-supplied VPS execution transcript, received and reviewed on 2026-09-23.

- delivery.log: race-enabled transport regressions, 20 repetitions, PASS.
- suite.log: complete Go suite, count=1, PASS.
- race.log: consensus, blockchain and node lifecycle, count=1, PASS.
- diagnostics.log: diagnostic capture tests, PASS.
- loopback.log: real four-node loopback, finality and MPR, PASS.

The earlier VPS RC2.1 netns run stalled at finalized height 46 under packet loss. The supplied RC3 VPS transcript now records PASS for all four-node gates:

- package SHA256 and all ten patch payload hashes: OK;
- lifecycle, delivery, complete suite, race and diagnostic tests: PASS;
- loopback finality 20, MPR finalized 42;
- netns finality 18, MPR finalized 39;
- packet loss: finalized 49;
- partition/reconnect: isolated node 3, finalized 79;
- proposer crash: node 1, finalized 110;
- HVM_NETWORK_RC3_FOUR_NODE_GATES_PASSED.

Run identifier: 20260923T071137Z; host: hpcVM46. This is user-supplied evidence, not an SSH execution by the reviewer. It establishes the reported four-node acceptance result, not public-network activation or a full security audit.

RC3 isolates per-peer outbound queues, bounds stream I/O deadlines, handles short writes and counts overload drops. It includes prior reactor resume/reproposal and lifecycle corrections. Signature checks, quorum and journaling remain enabled. Bounded queues may drop frames under overload; passing unit tests is not proof of liveness under network impairment.

## Reproduction

Use Go 1.25.7, bash, jq, curl, Python 3 and standard Linux utilities. Run from hvm-network/:

```sh
export GOTOOLCHAIN=local
go test ./... -count=1
go test -race ./consensus ./blockchain ./devnet/hvm-network/cmd/node -count=1
go test -race ./blockchain -run '^TestDelivery' -count=20
MODE=loopback NODES=4 bash devnet/hvm-network/run-all.sh
# On an isolated Linux test host with root and iproute2/tc, after loopback exits:
MODE=netns NODES=4 bash devnet/hvm-network/run-all.sh
```

Use one devnet run at a time. Netns uses hb3e-n* namespaces. Retain diagnostics from a failing run before rerunning. Do not run this harness against production data.

RC3 four-node acceptance evidence is attached in vps-acceptance.txt. Before public rollout, separately approve distinct HVM Testnet/Mainnet chain IDs, activation/migration compatibility and testnet rollout. Legacy/pre-HVM 1337 is not changed in this PR.

VPS transcript SHA256: 35f7fd3cbf665b46876a435ee18e63fc8a58bad98150e25ebed0051aa681cbc8
