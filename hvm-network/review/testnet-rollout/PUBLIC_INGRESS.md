# Public testnet ingress evidence

Operator output, 2026-09-26, package HashBurst-HVM-Ingress-v1.0.0:

- PUBLIC_HTTPS_FILTER_OK
- PUBLIC_WEBSOCKET_READ_ONLY_OK
- EVM_SUBSCRIPTIONS_NOT_IMPLEMENTED
- HVM_TESTNET_PUBLIC_READ_ONLY_INGRESS_OK

An initial local 404 during reload was followed by successful external tests.
This establishes filtered read-only HTTPS and WebSocket request/response.
Ethereum subscriptions, native application canary, EVM compatibility and
mainnet readiness are separate gates. The funded native canary is provided
in this change; its live execution evidence remains pending.
