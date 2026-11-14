#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
PUB="$ROOT/trust/release-ed25519.pub.pem"
EXPECTED="6ab39d078a95644e8efc569c307a9c373723b871c771d9381440c79aa6b7e10d"
ACTUAL="$(openssl pkey -pubin -in "$PUB" -outform DER | sha256sum | awk '{print $1}')"
test "$ACTUAL" = "$EXPECTED" || { echo "production release trust anchor fingerprint mismatch" >&2; exit 1; }
grep -qx "PUBLIC_KEY_DER_SHA256=$EXPECTED" "$ROOT/trust/PRODUCTION-RELEASE-SIGNING-IDENTITY.txt"
echo "RELEASE_SIGNING_PUBLIC_KEY_SHA256=$ACTUAL"
echo G5_PRODUCTION_SIGNING_IDENTITY_ANCHORED_OK
