# Testnet API and wallet acceptance

## This change
- hb_getFinalizedCommitment([integerHeight]) returns one stored BFT-finalized block's
  commitments and certificate. No latest alias, no unbounded range, no transactions.
- compare-finalized.py selects the minimum finalized height across four operator
  tunnels and compares the SAME height. Certificate signer subsets are not compared.
  This is agreement between trusted nodes, not standalone cryptographic QC validation.
- /ws is bounded read-only JSON-RPC request/response: 64 KiB request, 256 requests
  per connection, 30s idle read and 5s write limits, same-origin browser policy.
  Only explicit chain/finality reads are enabled; eth_subscribe is unsupported.
  Public ingress still needs connection/rate limits; /ws remains loopback only.
- prepare-observer-config.py prepares a separate observer config from a public
  approved testnet template and a NEW on-host P2P identity. No consensus key is read.
  Must run runtime --check after actual provisioning before starting it.

## On-host sequence after review and build
Build/test the runtime from this commit; preserve the approved config and journal.
Deploy first to a non-signing observer in a separate directory. Never use --provision
against existing validator state. For existing validators, binary rollout must be
one at a time, retaining their pin, data and signing journals.
Then run fixed-height comparison over four SSH tunnels. Do not repeat activation
or the completed v4 restart proof. This change has not been deployed on the VPS.

## Application canary
The devnet ctl canary depends on a fixture manifest and fixture recorder keys.
Do NOT run that fixture/bootstrap tool against the live persistent testnet.
A persistent canary needs an operator-controlled funded test account, local signing,
sequence lookup and private hb_sendRawTransactionV2 submission.
Verify deployment/call receipts, state changes, replay rejection and failed calls.
A key must never leave its host or be sent to an unlocked-account public endpoint.
No funded canary account or signed live transaction is established by this change.

## Full MetaMask target (not yet implemented)
The current HVM engine dispatches native contracts; RuntimeEVM naming alone is
not an EVM implementation. Complete compatibility requires a consensus-defined EVM
execution path, Ethereum address/value/nonce semantics, signed legacy and typed
transaction decoding/recovery, chain-ID replay protection, gas/fee rules,
eth_call, eth_estimateGas, eth_getCode, block/transaction/receipt/log RPC schemas,
filters and eth_subscribe/newHeads/logs with disconnect cleanup.
Implement these with deterministic execution/replay tests and a testnet activation
height. No silent change to already finalized blocks or to native HVM semantics.
Verify with an actual MetaMask browser: add/switch chain, balance/nonce, native
transfer, contract deployment/call, reverted transaction, receipt/log visibility.
Record supported wallet versions and exact test cases. HTTP wallet operation alone
does not prove WebSocket subscriptions, nor does a successful chain switch prove EVM.

## Mainnet gate
Legacy 1337 unchanged; testnet 4735490; mainnet 4735489 remains inactive.
No mainnet genesis, allocation, validators or signing keys are supplied here.
Mainnet needs its separate reviewed configuration after live testnet acceptance.
