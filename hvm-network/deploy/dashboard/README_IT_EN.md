# HVM Network dashboard V5 post-R9

blockchainapi.one/hashburst uses the English-only V5 card. HVM mainnet 4735489
(0x484201) is active after R9 production acceptance on 12 November 2026.
Testnet 4735490 and the legacy archive 1337 remain preserved.

V5 adds a GET-only mainnet status route backed exclusively by the local,
non-signing observer health endpoint on 127.0.0.1:18019. It does not expose
admin_peers, validator RPC administration, validator private keys, founder
keys, or remote validator access. P2P peers is the observer's live
peer_count; validator federation 4/4 is displayed separately because these
are different measurements.

On 64.31.4.9, after merge and package verification:

    sha256sum -c SHA256SUMS
    python3 test-dashboard.py
    node test-card.cjs
    python3 install-dashboard.py

The installer backs up the PHP page and nginx configuration, preserves the
existing CSP nonce, verifies PHP/nginx syntax, reloads nginx, and verifies
both mainnet status and public mainnet eth_chainId locally over HTTPS.
Unknown modified cards fail closed. It does not restart or modify blockchain,
consensus, APoW, validator, observer, gateway, wallet, or key services.
