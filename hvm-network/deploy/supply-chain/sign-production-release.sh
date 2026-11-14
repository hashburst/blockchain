#!/usr/bin/env bash
set -euo pipefail
DIR="${1:?release directory required}"
PRIV="${2:-/etc/hashburst-release/release-ed25519.pem}"
PUB="${3:-/etc/hashburst-release/release-ed25519.pub.pem}"
ROOT="$(cd "$(dirname "$0")" && pwd)"
test -s "$DIR/SHA256SUMS" || { echo "SHA256SUMS missing" >&2; exit 1; }
test -s "$PRIV" || { echo "production private key missing" >&2; exit 1; }
test -s "$PUB" || { echo "production public key missing" >&2; exit 1; }
EXPECTED="6ab39d078a95644e8efc569c307a9c373723b871c771d9381440c79aa6b7e10d"
ACTUAL="$(openssl pkey -pubin -in "$PUB" -outform DER | sha256sum | awk '{print $1}')"
test "$ACTUAL" = "$EXPECTED" || { echo "signer public key does not match anchored production identity" >&2; exit 1; }
openssl pkeyutl -sign -rawin -inkey "$PRIV" -in "$DIR/SHA256SUMS" -out "$DIR/SHA256SUMS.sig"
bash "$ROOT/verify-release.sh" "$DIR" "$ROOT/trust/release-ed25519.pub.pem"
echo G5_PRODUCTION_SIGNATURE_VERIFIED_OK
