# HVM Network dashboard

blockchainapi.one/hashburst uses the English-only card (V4_EN).
Legacy is displayed as 1337. Mainnet 4735489 remains not active.
The bilingual templates are retained as bilingual-card.html/js for hashburst.io;
no hashburst.io installer or deployment is implied.

On 64.31.4.9, run:

    sha256sum -c SHA256SUMS
    python3 test-dashboard.py
    python3 install-dashboard.py

The installer supports a fresh card and exact known V2/V3 upgrades. Unknown
modified cards are retained. It backs up the PHP page and nginx configuration,
preserves the existing CSP nonce, verifies PHP/nginx syntax and local HTTPS,
and restores its own files on failure. It does not restart blockchain nodes.
After installation inspect https://blockchainapi.one/hashburst/ and select
HVM Network. Refresh twice to check progress. The observer is not a validator
quorum certificate. No transaction or key is submitted by this card.
