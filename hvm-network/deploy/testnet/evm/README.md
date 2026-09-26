# EVM testnet activation gate

Source implementation and local integration tests are in PR #19. No production
activation height is selected. Do not add `protocol.evm` to deployed node.json:
that changes the consensus pin and the current runtime correctly rejects it.
A reviewed pin-transition procedure and four-node rollout are still required.

The `metamask-canary.html` file is a manual test harness, not a passed test report.
After EVM activation and HTTPS gateway deployment, serve it on an approved origin,
provide the verified Ethereum RPC URL and fund a separate MetaMask testnet account.
It permits only chain 4735490 and asks the wallet to sign three transactions:
self-transfer of one wei, deployment, then storage/log-producing contract call.
The JSON proof contains public data only. It does not contain validator keys.
Do not repeat a timed-out transaction blindly: first inspect its returned hash.

The public native-HVM endpoint currently deployed does not forward Ethereum writes
or subscriptions. The new loopback paths are `/evm` and `/evm/ws`; they are not
public endpoints yet. Legacy 1337 is unchanged. Mainnet 4735489 is not activated.
