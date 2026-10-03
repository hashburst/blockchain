# Failed v1 reactor recovery (testnet only)

The 2026-10-03 v1 journal shows exit 1 after a stale proposal validation at
151139/151140. RPC port 18009 disappeared because the process exited. This
procedure installs the reactor candidate compiled and tested at commit
2719806647f25012a8d6b868a645c0341b0442c1; it does not restart the faulty executable.

## Provenance

- CI run: 37123137869 (full Go suite, reactor race tests, regression fails on old reactor).
- Artifact: 11274157535, reactor-finality-candidate.
- ZIP SHA256: 91a763f4cb4f500a42f03c6a3dae093d963b4a01d3f4c2260fd13a5138279f32.
- Linux amd64 binary SHA256: 30a0cd390af000019577f06bcf97ce54959808aa4447ab318ef669cf8aa9be0e.
- Approved failed predecessor: 10cb427894e3f51c088be2eb06744706f9f080aa9a2877cda6944622ebb8e0b7.

The artifact requires glibc >= 2.34 (Ubuntu 22.04 provides 2.35). It is not a
portable static build or a macOS/Windows executable.

## Operator procedure

Use a clean, pinned checkout of the recovery PR #28. From the Mac run:

    caffeinate -i python3 CHECKOUT/hvm-network/deploy/runtime/recover-v1.py \
      --deployer-root /Users/marynalacava/Desktop/BI3339M-anteriorita/HashBurst_Deployer

Use the actual local directory spelling (including the accent in anteriorità).
The script verifies checkout files against Git, recovery-revision CI, the
candidate CI run and artifact digest. It downloads the existing tested binary;
no local rebuild or repetition of Go tests is required. GitHub credentials use
the existing deployer's prompt. SSH/scp authentication remains interactive.

Before requesting recovery it requires agreement and progress from v2, v3, v4
and the observer, plus matching v1 network/configuration identity. The stopped
v1 is never counted as a live vote; this is not a relaxation of the five-node
final acceptance. Commitments are compared, but QC signatures are not
independently verified by the Python coordinator.

Only v1's failed service may enter the explicit recovery path: PID 0, result
exit-code, status 1, exact approved predecessor and testnet configuration. Its
miner must remain stopped. The ordinary installer still requires a running
predecessor. Persistent worker phases preserve config, runtime pin and signing
journal prefixes. No ledger conversion, wallet allocation, IPFS change, miner
start or automatic rollback occurs.

Re-run the SAME recover-v1.py command after a transport timeout. If a worker is
active it is observed. Once start_requested exists, only verification runs;
no second start is issued. A stopped worker at an earlier phase requires
inspection; the script refuses to invent completion or blindly restart it.

On success expect:

    V1_RECOVERY_AND_FIVE_NODE_PROGRESS_OK
    REPORT=.../ACCEPTANCE.json

The report records current five-node common-height agreement and progress,
v1's exact new binary hash, and the preserved installer evidence on the VPS.
No evidence is written into the original deployment.json. Do not run the OLD
deploy/publish commands after this recovery: the fleet now intentionally has
v1 on a new runtime and the remaining nodes on the previous candidate. Review
ACCEPTANCE.json before preparing the next pinned fleet release. PR #27, its
pinned installer-only wrapper, tags, mainnet and both sites are unchanged.

## Validation

`test_recover_v1.py` covers explicit stopped-node authorization, wrong host/hash/
failure exclusions, manifest binding, cross-origin token stripping and
commitment disagreement. `test-systemd-failed-v1.sh` exercises a real persistent
systemd job on a disposable runner using a fake health server (not a consensus
proof), including resume without a second node start.
