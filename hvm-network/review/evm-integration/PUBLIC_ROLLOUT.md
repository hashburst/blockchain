# Public rollout implementation evidence

26 September 2026. This report records local implementation/tests only.

Added offline --migrate-evm: exclusive runtime lock, only first EVM testnet
configuration addition, future-height guard, candidate replay, immutable intent,
resumable pin/config replacement preserving owners, identity and journal bytes.
Tests cover successful/idempotent migration, interruption between pin/config,
live-lock rejection, identity/chain changes, too-close height and changed intent.

Added separate HTTP/WS public gateway and Nginx installer. Tests exercise forbidden
methods/batches/notifications, cross-origin HTTP for wallets, WebSocket subscription
limit and upstream cleanup. Race detector passes for gateway and runtime packages.
Added read-only plan creation, five-node stop/migrate/start barrier, common-height
commitment comparison, explicit single-v4 restart and journal-prefix checks.
Python common-height and wrong-network gate tests pass.

Full node go test ./... and go vet ./... pass. Static linux/amd64 builds for runtime
and gateway pass with Go 1.25.7, CGO_ENABLED=0 and -buildvcs=false. JavaScript syntax
check passes. No live SSH authentication or wallet session was available here.

The MetaMask harness now exercises newHeads and logs, receipt/event agreement,
unsubscribe, and exports public evidence while retaining submitted hashes on error.
Actual MetaMask execution has NOT been performed. Four VPS plus observer EVM rollout
has NOT been performed. Public EVM ingress has NOT been installed in this session.
Mainnet 4735489 remains inactive. Legacy 1337 and deployed state/journals unchanged.

Known scope: fixed base fee, no arbitrary historical account-state snapshots/state
overrides; wallet compatibility must be stated for tested versions/scenarios only.
The live comparison relies on runtime certificate validation rather than a separate
cryptographic proof verifier. Review these limits before public release.
