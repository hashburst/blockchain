# HVM testnet public ingress and application acceptance

## Intended routes (not deployed)

- https://blockchainapi.one/api/hashburst/hvm/testnet/health
- https://blockchainapi.one/api/hashburst/hvm/testnet/rpc

Validator RPC remains on loopback port 18009. Do not proxy to public-IP:18009:
that endpoint is not listening publicly. Provision a separate non-signing observer
on the ingress host, with the testnet registry/configuration and its own peer key,
then connect it to the four validators on TCP 31307. Do not copy validator keys.
Verify the observer catches up before connecting Nginx to its local RPC.
Inventory only the ingress host's relevant ports/configuration before installation.

The existing runtime exposes /health and /rpc. A public JSON-RPC method filter
must permit explicitly reviewed read methods first, with body-size, rate and
connection limits. Keep hb_sendTransaction disabled publicly. Signed transaction
submission hb_sendRawTransactionV2 is enabled only after the private canary passes.
Do not publish a WebSocket endpoint: this runtime has no verified WS implementation.
Do not claim complete MetaMask/Ethereum compatibility: eth_sendRawTransaction and
eth_getBlockByNumber are absent from the implemented method switch.

## Acceptance order

1. Fixed-height agreement: compare block hash, HVM/HBT roots and finality certificate
   at one chosen finalized height across all four nodes. Current RPC lacks the
   required historical block query; add a bounded read-only query with tests first.
2. Private application canary: authorized test account submits an HVM deployment,
   calls it, observes a successful receipt and verifies resulting state. Test a
   rejected transaction and replay rejection. Never print signing keys.
3. Non-signing ingress observer catches up; loopback smoke test verifies chain ID,
   finalized progress, and JSON-RPC error handling.
4. Nginx TLS publication, then repeat external smoke/canary through the filtered API.
5. Soak: finality, message-drop deltas, restart readiness, resource/disk growth.
6. Mainnet-specific genesis/registry/keys/configuration and activation review.

## Mainnet

ID 4735489 is assigned but not active. Reuse no testnet keys, checkpoint or signing
journals. Preserve legacy 1337 and its public genesis. Mainnet genesis allocation,
validator identities/powers and operational endpoints require a separate concrete
configuration. Public testnet acceptance precedes activation.
