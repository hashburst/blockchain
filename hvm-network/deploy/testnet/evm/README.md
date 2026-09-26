# Controlled EVM testnet activation

This is deployment tooling, not a report of live deployment. Testnet 4735490 only;
legacy 1337 and mainnet 4735489 are unchanged. Never copy a validator private key,
restore an earlier journal, reprovision the checkpoint, or delete runtime.pin.

## Build and plan

Use the reviewed PR revision and Go 1.25.7. Run verify-local.sh, then
`bash hvm-network/deploy/testnet/evm/build-release.sh /absolute/release-directory`.
Run `python3 test-rollout.py` in that directory. Check SHA256SUMS before installing.
The generated SOURCE_COMMIT must identify the reviewed source, not a dirty tree.

Generate a read-only candidate plan with `python3 make-plan.py --binary hashburst-testnet --out plan.json`. It reads the five live heights and checks the shared configuration. Defaults are a 10000-block margin, gas limit 1000000 and fixed base fee 1 wei; the printed plan makes these explicit before any service change. Adjust these testnet parameters with the documented CLI options if the reviewed network decision differs. Review these fields:

- chain_id: 4735490
- binary_sha256: SHA256 of the built hashburst-testnet
- observer_node_id: exact node_id from the deployed observer configuration
- evm: activation_height, gas_limit, base_fee_wei

Activation height must exceed the highest finalized height on all five nodes by
more than 1000 blocks. Choose sufficient margin for recovery; do not infer a
current height from old logs. Gas limit and fixed base fee are consensus decisions;
all five nodes must receive the identical plan. Initial implementation supports
Cancun execution and a fixed base fee, not adaptive EIP-1559 base-fee economics.

## Migration and live gates

From a machine with SSH access to all five VPS:

```
python3 rollout.py activate --plan plan.json --binary hashburst-testnet
python3 rollout.py restart-v4 --plan plan.json
```

The first command checks agreement at a common finalized height, stages the same
binary on every node, then deliberately stops all five testnet services before
changing consensus parameters. This is a coordinated testnet maintenance window,
not a zero-downtime upgrade. Legacy services are never stopped.

The runtime's --migrate-evm command takes its exclusive data lock, checks the old
pin, identity, private-key binding, registry and journals, replays existing history
under the candidate parameters and permits only adding protocol.evm. It records an
immutable intent before replacing pin/config atomically one file at a time. An
interruption between writes prevents ordinary startup; repeat the SAME migration
candidate to complete it. File owners/modes and signing-journal bytes are retained.
After all offline migrations succeed, a barrier starts the five services together.

On failure, services/state/journals are retained; inspect the printed evidence.
If migration was incomplete and services are stopped, use the SAME plan and binary
with `rollout.py resume-migration --plan plan.json --binary hashburst-testnet`.
If startup completed, use `rollout.py check --plan plan.json` (no restart).
The explicit restart-v4 command restarts only v4 after a successful common-height
progress check. Do not rerun restart-v4 merely to poll recovery.

Readiness waits are bounded at two hours. Commitments compare chain ID, block and
parent hash, native roots, validator root, EVM state/receipt roots and gas at one
fixed finalized height, with nonempty node-validated certificates. Different
certificate signer subsets do not imply different blocks. Evidence also verifies
all original journal prefixes and that the observer signing journals remain empty.
These scripts trust each runtime's certificate verification; they are not a second
independent cryptographic verifier. Finality must progress after restart; the restart gate also requires a new local precommit beyond the pre-restart journal height and an unchanged runtime pin. An existing restart marker prevents an accidental second restart.

## Public ingress

Only after both live gates pass, copy the release directory to 64.31.4.9, verify
SHA256SUMS and run `python3 install-ingress.py`. It requires the existing unsigned
observer to have finalized EVM activation. It retains native public routes and
installs a separate loopback gateway on 18011 plus Nginx routes:

- HTTPS RPC: https://blockchainapi.one/api/hashburst/hvm/testnet/evm
- WSS: wss://blockchainapi.one/api/hashburst/hvm/testnet/evm/ws
- Canary: https://blockchainapi.one/hvm-testnet-metamask/

Only signed raw transactions are admitted over HTTP. No server signing, personal,
admin, debug, native writes, batches or notifications. WebSocket permits reads,
newHeads/log subscriptions and unsubscribe; no writes. Limits: 16 concurrent gateway
operations/connections, 8 subscriptions and 16 pending requests per WS connection,
5 messages/second with burst 10, 270000-byte input, 2 MiB output, 10-minute WS
lifetime. Nginx also limits per-IP requests/connections; gateway enforces message
limits after upgrade. HTTP CORS is public; WS browser origin is blockchainapi.one.
Reconnect and use historical logs for catch-up. Never bypass the gateway to expose
the runtime's WebSocket directly.

## Actual MetaMask acceptance

Fund a separate testnet account with test HBT using the approved funding path;
never import validator/operator keys into a browser. Open the HTTPS canary with
MetaMask installed. It checks chain ID, asks permission to add/switch network and
connect, then requests three wallet confirmations: transfer, deploy and contract
call. It verifies receipts, block hashes, storage 42, historical logs, matching
newHeads/log notifications and successful unsubscription, and downloads public JSON
proof. Rejections/timeouts retain submitted transaction hashes. A completed tool
run is evidence for that browser/wallet version and these scenarios, not universal
MetaMask or Ethereum compatibility. Record wallet/browser versions with the proof.

Actual VPS execution and MetaMask confirmation must appear in review evidence
before removing the public-rollout gate. Local tests must never be labeled live
acceptance. Historical account-state queries and state overrides remain unsupported.
Mainnet requires its own configuration, release decision and rollout.
